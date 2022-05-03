// Package policy implements the application of a declarative configuration
// for Vault policies.
package policy

import (
	"sync"

	"github.com/app-sre/vault-manager/pkg/utils"
	"github.com/app-sre/vault-manager/pkg/vault"
	"github.com/app-sre/vault-manager/toplevel"
	"github.com/app-sre/vault-manager/toplevel/instance"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

type config struct{}

var _ toplevel.Configuration = config{}

func init() {
	toplevel.RegisterConfiguration("vault_policies", config{})
}

type entry struct {
	Name        string            `yaml:"name"`
	Rules       string            `yaml:"rules"`
	Type        string            `yaml:"type"`
	Instance    instance.Instance `yaml:"instance"`
	Description string            `yaml:"description"`
}

var _ vault.Item = entry{}

func (e entry) Key() string {
	return e.Name
}

func (e entry) KeyForType() string {
	return e.Type
}

func (e entry) KeyForDescription() string {
	return e.Description
}

func (e entry) Equals(i interface{}) bool {
	entry, ok := i.(entry)
	if !ok {
		return false
	}

	return e.Name == entry.Name && e.Rules == entry.Rules
}

func (c config) Apply(entriesBytes []byte, dryRun bool, threadPoolSize int) {
	// Unmarshal the list of configured secrets engines.
	var entries []entry
	if err := yaml.Unmarshal(entriesBytes, &entries); err != nil {
		log.WithError(err).Fatal("[Vault Policy] failed to decode policies configuration")
	}
	instancesToDesiredPolicies := make(map[string][]entry)
	for _, e := range entries {
		instancesToDesiredPolicies[e.Instance.Address] = append(instancesToDesiredPolicies[e.Instance.Address], e)
	}

	// call to vault api for each instance to obtain raw enabled audit info
	instancesToExistingPolicyNames := make(map[string][]string)
	for _, e := range entries {
		if _, exists := instancesToExistingPolicyNames[e.Instance.Address]; !exists {
			instancesToExistingPolicyNames[e.Instance.Address] =
				append(instancesToExistingPolicyNames[e.Instance.Address], vault.ListVaultPolicies(e.Instance.Address)...)
		}
	}

	// Build a list of all the existing policies for each instance
	instancesToExistingPolicies := make(map[string][]entry)
	for instance, existingPolicyNames := range instancesToExistingPolicyNames {
		if existingPolicyNames != nil {
			var mutex = &sync.Mutex{}
			bwg := utils.NewBoundedWaitGroup(threadPoolSize)

			// fill existing policies array in parallel
			for i := range existingPolicyNames {
				bwg.Add(1)

				go func(i int) {
					name := existingPolicyNames[i]
					policy := vault.GetVaultPolicy(instance, name)

					mutex.Lock()
					instancesToExistingPolicies[instance] =
						append(instancesToExistingPolicies[instance], entry{Name: name, Rules: policy})

					defer bwg.Done()
					defer mutex.Unlock()
				}(i)
			}
			bwg.Wait()
		}
	}

	// perform reconcile operations for each instance
	for _, instance := range instance.InstanceAddresses {
		// Diff the local configuration with the Vault instance.
		toBeWritten, toBeDeleted, _ :=
			vault.DiffItems(asItems(instancesToDesiredPolicies[instance]), asItems(instancesToExistingPolicies[instance]))

		if dryRun == true {
			for _, w := range toBeWritten {
				log.Infof("[Dry Run] [Vault Policy] policy to be written='%v'", w.Key())
			}
			for _, d := range toBeDeleted {
				if isDefaultPolicy(d.Key()) {
					continue
				}

				log.Infof("[Dry Run] [Vault Policy] policy to be deleted='%v'", d.Key())
			}
		} else {
			// Write any missing policies to the Vault instance.
			for _, e := range toBeWritten {
				ent := e.(entry)
				vault.PutVaultPolicy(instance, ent.Name, ent.Rules)
			}
			// Delete any policies from the Vault instance.
			for _, e := range toBeDeleted {
				ent := e.(entry)
				if isDefaultPolicy(ent.Name) {
					continue
				}
				vault.DeleteVaultPolicy(instance, ent.Name)
			}
		}
	}
}

func isDefaultPolicy(name string) bool {
	return name == "root" || name == "default"
}

func asItems(xs []entry) (items []vault.Item) {
	items = make([]vault.Item, 0)
	for _, x := range xs {
		items = append(items, x)
	}

	return
}
