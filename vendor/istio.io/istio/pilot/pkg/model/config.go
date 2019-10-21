// Copyright 2017 Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package model

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mitchellh/copystructure"

	"github.com/gogo/protobuf/proto"

	mccpb "istio.io/api/mixer/v1/config/client"
	networking "istio.io/api/networking/v1alpha3"

	"istio.io/istio/pkg/config/constants"
	"istio.io/istio/pkg/config/host"
	"istio.io/istio/pkg/config/labels"
	"istio.io/istio/pkg/config/schema"
	"istio.io/istio/pkg/config/schemas"
)

// ConfigMeta is metadata attached to each configuration unit.
// The revision is optional, and if provided, identifies the
// last update operation on the object.
type ConfigMeta struct {
	// Type is a short configuration name that matches the content message type
	// (e.g. "route-rule")
	Type string `json:"type,omitempty"`

	// Group is the API group of the config.
	Group string `json:"group,omitempty"`

	// Version is the API version of the Config.
	Version string `json:"version,omitempty"`

	// Name is a unique immutable identifier in a namespace
	Name string `json:"name,omitempty"`

	// Namespace defines the space for names (optional for some types),
	// applications may choose to use namespaces for a variety of purposes
	// (security domains, fault domains, organizational domains)
	Namespace string `json:"namespace,omitempty"`

	// Domain defines the suffix of the fully qualified name past the namespace.
	// Domain is not a part of the unique key unlike name and namespace.
	Domain string `json:"domain,omitempty"`

	// Map of string keys and values that can be used to organize and categorize
	// (scope and select) objects.
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations is an unstructured key value map stored with a resource that may be
	// set by external tools to store and retrieve arbitrary metadata. They are not
	// queryable and should be preserved when modifying objects.
	Annotations map[string]string `json:"annotations,omitempty"`

	// ResourceVersion is an opaque identifier for tracking updates to the config registry.
	// The implementation may use a change index or a commit log for the revision.
	// The config client should not make any assumptions about revisions and rely only on
	// exact equality to implement optimistic concurrency of read-write operations.
	//
	// The lifetime of an object of a particular revision depends on the underlying data store.
	// The data store may compactify old revisions in the interest of storage optimization.
	//
	// An empty revision carries a special meaning that the associated object has
	// not been stored and assigned a revision.
	ResourceVersion string `json:"resourceVersion,omitempty"`

	// CreationTimestamp records the creation time
	CreationTimestamp time.Time `json:"creationTimestamp,omitempty"`
}

// Config is a configuration unit consisting of the type of configuration, the
// key identifier that is unique per type, and the content represented as a
// protobuf message.
type Config struct {
	ConfigMeta

	// Spec holds the configuration object as a gogo protobuf message
	Spec proto.Message
}

// ConfigStore describes a set of platform agnostic APIs that must be supported
// by the underlying platform to store and retrieve Istio configuration.
//
// Configuration key is defined to be a combination of the type, name, and
// namespace of the configuration object. The configuration key is guaranteed
// to be unique in the store.
//
// The storage interface presented here assumes that the underlying storage
// layer supports _Get_ (list), _Update_ (update), _Create_ (create) and
// _Delete_ semantics but does not guarantee any transactional semantics.
//
// _Update_, _Create_, and _Delete_ are mutator operations. These operations
// are asynchronous, and you might not see the effect immediately (e.g. _Get_
// might not return the object by key immediately after you mutate the store.)
// Intermittent errors might occur even though the operation succeeds, so you
// should always check if the object store has been modified even if the
// mutating operation returns an error.  Objects should be created with
// _Create_ operation and updated with _Update_ operation.
//
// Resource versions record the last mutation operation on each object. If a
// mutation is applied to a different revision of an object than what the
// underlying storage expects as defined by pure equality, the operation is
// blocked.  The client of this interface should not make assumptions about the
// structure or ordering of the revision identifier.
//
// Object references supplied and returned from this interface should be
// treated as read-only. Modifying them violates thread-safety.
type ConfigStore interface {
	// ConfigDescriptor exposes the configuration type schema known by the config store.
	// The type schema defines the bidrectional mapping between configuration
	// types and the protobuf encoding schema.
	ConfigDescriptor() schema.Set

	// Get retrieves a configuration element by a type and a key
	Get(typ, name, namespace string) *Config

	// List returns objects by type and namespace.
	// Use "" for the namespace to list across namespaces.
	List(typ, namespace string) ([]Config, error)

	// Create adds a new configuration object to the store. If an object with the
	// same name and namespace for the type already exists, the operation fails
	// with no side effects.
	Create(config Config) (revision string, err error)

	// Update modifies an existing configuration object in the store.  Update
	// requires that the object has been created.  Resource version prevents
	// overriding a value that has been changed between prior _Get_ and _Put_
	// operation to achieve optimistic concurrency. This method returns a new
	// revision if the operation succeeds.
	Update(config Config) (newRevision string, err error)

	// Delete removes an object from the store by key
	Delete(typ, name, namespace string) error

	Version() string

	GetResourceAtVersion(version string, key string) (resourceVersion string, err error)
}

// Key function for the configuration objects
func Key(typ, name, namespace string) string {
	return fmt.Sprintf("%s/%s/%s", typ, namespace, name)
}

// Key is the unique identifier for a configuration object
func (meta *ConfigMeta) Key() string {
	return Key(meta.Type, meta.Name, meta.Namespace)
}

// ConfigStoreCache is a local fully-replicated cache of the config store.  The
// cache actively synchronizes its local state with the remote store and
// provides a notification mechanism to receive update events. As such, the
// notification handlers must be registered prior to calling _Run_, and the
// cache requires initial synchronization grace period after calling  _Run_.
//
// Update notifications require the following consistency guarantee: the view
// in the cache must be AT LEAST as fresh as the moment notification arrives, but
// MAY BE more fresh (e.g. if _Delete_ cancels an _Add_ event).
//
// Handlers execute on the single worker queue in the order they are appended.
// Handlers receive the notification event and the associated object.  Note
// that all handlers must be registered before starting the cache controller.
//go:generate counterfeiter -o ../config/aggregate/fakes/config_store_cache.gen.go --fake-name ConfigStoreCache . ConfigStoreCache
type ConfigStoreCache interface {
	ConfigStore

	// RegisterEventHandler adds a handler to receive config update events for a
	// configuration type
	RegisterEventHandler(typ string, handler func(Config, Event))

	// Run until a signal is received
	Run(stop <-chan struct{})

	// HasSynced returns true after initial cache synchronization is complete
	HasSynced() bool
}

// IstioConfigStore is a specialized interface to access config store using
// Istio configuration types
// nolint
//go:generate counterfeiter -o ../networking/core/v1alpha3/fakes/fake_istio_config_store.gen.go --fake-name IstioConfigStore . IstioConfigStore
type IstioConfigStore interface {
	ConfigStore

	// ServiceEntries lists all service entries
	ServiceEntries() []Config

	// Gateways lists all gateways bound to the specified workload labels
	Gateways(workloadLabels labels.Collection) []Config

	// EnvoyFilter lists the envoy filter configuration bound to the specified workload labels
	EnvoyFilter(workloadLabels labels.Collection) *Config

	// QuotaSpecByDestination selects Mixerclient quota specifications
	// associated with destination service instances.
	QuotaSpecByDestination(instance *ServiceInstance) []Config

	// ServiceRoles selects ServiceRoles in the specified namespace.
	ServiceRoles(namespace string) []Config

	// ServiceRoleBindings selects ServiceRoleBindings in the specified namespace.
	ServiceRoleBindings(namespace string) []Config

	// RbacConfig selects the RbacConfig of name DefaultRbacConfigName.
	RbacConfig() *Config

	// ClusterRbacConfig selects the ClusterRbacConfig of name DefaultRbacConfigName.
	ClusterRbacConfig() *Config

	// AuthorizationPolicies selects AuthorizationPolicies in the specified namespace.
	AuthorizationPolicies(namespace string) []Config
}

const (
	// NamespaceAll is a designated symbol for listing across all namespaces
	NamespaceAll = ""
)

/*
  This conversion of CRD (== yaml files with k8s metadata) is extremely inefficient.
  The yaml is parsed (kubeyaml), converted to YAML again (FromJSONMap),
  converted to JSON (YAMLToJSON) and finally UnmarshallString in proto is called.

  The result is not cached in the model.

  In 0.7, this was the biggest factor in scalability. Moving forward we will likely
  deprecate model, and do the conversion (hopefully more efficient) only once, when
  an object is first read.
*/

// ResolveHostname produces a FQDN based on either the service or
// a concat of the namespace + domain
// Deprecated. Do not use
func ResolveHostname(meta ConfigMeta, svc *mccpb.IstioService) host.Name {
	out := svc.Name
	// if FQDN is specified, do not append domain or namespace to hostname
	// Service field has precedence over Name
	if svc.Service != "" {
		out = svc.Service
	} else {
		if svc.Namespace != "" {
			out = out + "." + svc.Namespace
		} else if meta.Namespace != "" {
			out = out + "." + meta.Namespace
		}

		if svc.Domain != "" {
			out = out + "." + svc.Domain
		} else if meta.Domain != "" {
			out = out + ".svc." + meta.Domain
		}
	}

	return host.Name(out)
}

// ResolveShortnameToFQDN uses metadata information to resolve a reference
// to shortname of the service to FQDN
func ResolveShortnameToFQDN(hostname string, meta ConfigMeta) host.Name {
	out := hostname
	// Treat the wildcard hostname as fully qualified. Any other variant of a wildcard hostname will contain a `.` too,
	// and skip the next if, so we only need to check for the literal wildcard itself.
	if hostname == "*" {
		return host.Name(out)
	}
	// if FQDN is specified, do not append domain or namespace to hostname
	if !strings.Contains(hostname, ".") {
		if meta.Namespace != "" {
			out = out + "." + meta.Namespace
		}

		// FIXME this is a gross hack to hardcode a service's domain name in kubernetes
		// BUG this will break non kubernetes environments if they use shortnames in the
		// rules.
		if meta.Domain != "" {
			out = out + ".svc." + meta.Domain
		}
	}

	return host.Name(out)
}

// resolveGatewayName uses metadata information to resolve a reference
// to shortname of the gateway to FQDN
func resolveGatewayName(gwname string, meta ConfigMeta) string {
	out := gwname

	// New way of binding to a gateway in remote namespace
	// is ns/name. Old way is either FQDN or short name
	if !strings.Contains(gwname, "/") {
		if !strings.Contains(gwname, ".") {
			// we have a short name. Resolve to a gateway in same namespace
			out = fmt.Sprintf("%s/%s", meta.Namespace, gwname)
		} else {
			// parse namespace from FQDN. This is very hacky, but meant for backward compatibility only
			parts := strings.Split(gwname, ".")
			out = fmt.Sprintf("%s/%s", parts[1], parts[0])
		}
	} else {
		// remove the . from ./gateway and substitute it with the namespace name
		parts := strings.Split(gwname, "/")
		if parts[0] == "." {
			out = fmt.Sprintf("%s/%s", meta.Namespace, parts[1])
		}
	}
	return out
}

// MostSpecificHostMatch compares the elements of the stack to the needle, and returns the longest stack element
// matching the needle, or false if no element in the stack matches the needle.
func MostSpecificHostMatch(needle host.Name, stack []host.Name) (host.Name, bool) {
	for _, h := range stack {
		if needle.Matches(h) {
			return h, true
		}
	}
	return "", false
}

// istioConfigStore provides a simple adapter for Istio configuration types
// from the generic config registry
type istioConfigStore struct {
	ConfigStore
}

// MakeIstioStore creates a wrapper around a store.
// In pilot it is initialized with a ConfigStoreCache, tests only use
// a regular ConfigStore.
func MakeIstioStore(store ConfigStore) IstioConfigStore {
	return &istioConfigStore{store}
}

func (store *istioConfigStore) ServiceEntries() []Config {
	serviceEntries, err := store.List(schemas.ServiceEntry.Type, NamespaceAll)
	if err != nil {
		return nil
	}
	supportedTypes := store.ConfigDescriptor()
	if _, ok := supportedTypes.GetByType(schemas.SyntheticServiceEntry.Type); ok {
		syntheticServiceEntries, err := store.List(schemas.SyntheticServiceEntry.Type, NamespaceAll)
		if err != nil {
			return nil
		}
		return append(serviceEntries, syntheticServiceEntries...)

	}
	return serviceEntries
}

// sortConfigByCreationTime sorts the list of config objects in ascending order by their creation time (if available).
func sortConfigByCreationTime(configs []Config) []Config {
	sort.SliceStable(configs, func(i, j int) bool {
		// If creation time is the same, then behavior is nondeterministic. In this case, we can
		// pick an arbitrary but consistent ordering based on name and namespace, which is unique.
		// CreationTimestamp is stored in seconds, so this is not uncommon.
		if configs[i].CreationTimestamp == configs[j].CreationTimestamp {
			in := configs[i].Name + "." + configs[i].Namespace
			jn := configs[j].Name + "." + configs[j].Namespace
			return in < jn
		}
		return configs[i].CreationTimestamp.Before(configs[j].CreationTimestamp)
	})
	return configs
}

func (store *istioConfigStore) Gateways(workloadLabels labels.Collection) []Config {
	configs, err := store.List(schemas.Gateway.Type, NamespaceAll)
	if err != nil {
		return nil
	}

	sortConfigByCreationTime(configs)
	out := make([]Config, 0)
	for _, cfg := range configs {
		gateway := cfg.Spec.(*networking.Gateway)
		if gateway.GetSelector() == nil {
			// no selector. Applies to all workloads asking for the gateway
			out = append(out, cfg)
		} else {
			gatewaySelector := labels.Instance(gateway.GetSelector())
			if workloadLabels.IsSupersetOf(gatewaySelector) {
				out = append(out, cfg)
			}
		}
	}
	return out
}

func (store *istioConfigStore) EnvoyFilter(workloadLabels labels.Collection) *Config {
	configs, err := store.List(schemas.EnvoyFilter.Type, NamespaceAll)
	if err != nil {
		return nil
	}

	sortConfigByCreationTime(configs)

	// When there are multiple envoy filter configurations for a workload
	// merge them instead of randomly picking one
	mergedFilterConfig := &networking.EnvoyFilter{}

	for _, cfg := range configs {
		filter := cfg.Spec.(*networking.EnvoyFilter)
		// if there is no workload selector, the filter applies to all workloads
		// if there is a workload selector, check for matching workload labels
		if filter.WorkloadLabels != nil {
			workloadSelector := labels.Instance(filter.WorkloadLabels)
			if !workloadLabels.IsSupersetOf(workloadSelector) {
				continue
			}
		}
		mergedFilterConfig.Filters = append(mergedFilterConfig.Filters, filter.Filters...)
	}

	return &Config{Spec: mergedFilterConfig}
}

// matchWildcardService matches destinationHost to a wildcarded svc.
// checked values for svc
//     '*'  matches everything
//     '*.ns.*'  matches anything in the same namespace
//		strings of any other form are not matched.
func matchWildcardService(destinationHost, svc string) bool {
	if len(svc) == 0 || !strings.Contains(svc, "*") {
		return false
	}

	if svc == "*" {
		return true
	}

	// check for namespace match with svc like '*.ns.*'
	// extract match substring by dropping '*'
	if strings.HasPrefix(svc, "*") && strings.HasSuffix(svc, "*") {
		return strings.Contains(destinationHost, svc[1:len(svc)-1])
	}

	log.Warnf("Wildcard pattern '%s' is not allowed. Only '*' or '*.<ns>.*' is allowed.", svc)

	return false
}

// MatchesDestHost returns true if the service instance matches the given IstioService
// ex: binding host(details.istio-system.svc.cluster.local) ?= instance(reviews.default.svc.cluster.local)
func MatchesDestHost(destinationHost string, meta ConfigMeta, svc *mccpb.IstioService) bool {
	if matchWildcardService(destinationHost, svc.Service) {
		return true
	}

	// try exact matches
	hostname := string(ResolveHostname(meta, svc))
	if destinationHost == hostname {
		return true
	}
	shortName := hostname[0:strings.Index(hostname, ".")]
	if strings.HasPrefix(destinationHost, shortName) {
		log.Warnf("Quota excluded. service: %s matches binding shortname: %s, but does not match fqdn: %s",
			destinationHost, shortName, hostname)
	}

	return false
}

func recordSpecRef(refs map[string]bool, bindingNamespace string, quotas []*mccpb.QuotaSpecBinding_QuotaSpecReference) {
	for _, spec := range quotas {
		namespace := spec.Namespace
		if namespace == "" {
			namespace = bindingNamespace
		}
		refs[key(spec.Name, namespace)] = true
	}
}

// key creates a key from a reference's name and namespace.
func key(name, namespace string) string {
	return name + "/" + namespace
}

// findQuotaSpecRefs returns a set of quotaSpec reference names
func findQuotaSpecRefs(instance *ServiceInstance, bindings []Config) map[string]bool {
	// Build the set of quota spec references bound to the service instance.
	refs := make(map[string]bool)
	for _, binding := range bindings {
		b := binding.Spec.(*mccpb.QuotaSpecBinding)
		for _, service := range b.Services {
			if MatchesDestHost(string(instance.Service.Hostname), binding.ConfigMeta, service) {
				recordSpecRef(refs, binding.Namespace, b.QuotaSpecs)
				// found a binding that matches the instance.
				break
			}
		}
	}

	return refs
}

// QuotaSpecByDestination selects Mixerclient quota specifications
// associated with destination service instances.
func (store *istioConfigStore) QuotaSpecByDestination(instance *ServiceInstance) []Config {
	log.Debugf("QuotaSpecByDestination(%v)", instance)
	bindings, err := store.List(schemas.QuotaSpecBinding.Type, NamespaceAll)
	if err != nil {
		log.Warnf("Unable to fetch QuotaSpecBindings: %v", err)
		return nil
	}

	log.Debugf("QuotaSpecByDestination bindings[%d] %v", len(bindings), bindings)
	specs, err := store.List(schemas.QuotaSpec.Type, NamespaceAll)
	if err != nil {
		log.Warnf("Unable to fetch QuotaSpecs: %v", err)
		return nil
	}

	log.Debugf("QuotaSpecByDestination specs[%d] %v", len(specs), specs)

	// Build the set of quota spec references bound to the service instance.
	refs := findQuotaSpecRefs(instance, bindings)
	log.Debugf("QuotaSpecByDestination refs:%v", refs)

	// Append any spec that is in the set of references.
	// Remove matching specs from refs so refs only contains dangling references.
	var out []Config
	for _, spec := range specs {
		refkey := key(spec.ConfigMeta.Name, spec.ConfigMeta.Namespace)
		if refs[refkey] {
			out = append(out, spec)
			delete(refs, refkey)
		}
	}

	if len(refs) > 0 {
		log.Warnf("Some matched QuotaSpecs were not found: %v", refs)
	}
	return out
}

func (store *istioConfigStore) ServiceRoles(namespace string) []Config {
	roles, err := store.List(schemas.ServiceRole.Type, namespace)
	if err != nil {
		log.Errorf("failed to get ServiceRoles in namespace %s: %v", namespace, err)
		return nil
	}

	return roles
}

func (store *istioConfigStore) ServiceRoleBindings(namespace string) []Config {
	bindings, err := store.List(schemas.ServiceRoleBinding.Type, namespace)
	if err != nil {
		log.Errorf("failed to get ServiceRoleBinding in namespace %s: %v", namespace, err)
		return nil
	}

	return bindings
}

func (store *istioConfigStore) ClusterRbacConfig() *Config {
	clusterRbacConfig, err := store.List(schemas.ClusterRbacConfig.Type, "")
	if err != nil {
		log.Errorf("failed to get ClusterRbacConfig: %v", err)
	}
	for _, rc := range clusterRbacConfig {
		if rc.Name == constants.DefaultRbacConfigName {
			return &rc
		}
	}
	return nil
}

func (store *istioConfigStore) RbacConfig() *Config {
	rbacConfigs, err := store.List(schemas.RbacConfig.Type, "")
	if err != nil {
		return nil
	}

	if len(rbacConfigs) > 1 {
		log.Errorf("found %d RbacConfigs, expecting only 1.", len(rbacConfigs))
	}
	for _, rc := range rbacConfigs {
		if rc.Name == constants.DefaultRbacConfigName {
			log.Warnf("RbacConfig is deprecated, Use ClusterRbacConfig instead.")
			return &rc
		}
	}
	return nil
}

func (store *istioConfigStore) AuthorizationPolicies(namespace string) []Config {
	authorizationPolicies, err := store.List(schemas.AuthorizationPolicy.Type, namespace)
	if err != nil {
		log.Errorf("failed to get AuthorizationPolicy in namespace %s: %v", namespace, err)
		return nil
	}

	return authorizationPolicies
}

// SortQuotaSpec sorts a slice in a stable manner.
func SortQuotaSpec(specs []Config) {
	sort.Slice(specs, func(i, j int) bool {
		// protect against incompatible types
		irule, _ := specs[i].Spec.(*mccpb.QuotaSpec)
		jrule, _ := specs[j].Spec.(*mccpb.QuotaSpec)
		return irule == nil || jrule == nil || (specs[i].Key() < specs[j].Key())
	})
}

func (config Config) DeepCopy() Config {
	copied, err := copystructure.Copy(config)
	if err != nil {
		// There are 2 locations where errors are generated in copystructure.Copy:
		//  * The reflection walk over the structure fails, which should never happen
		//  * A configurable copy function returns an error. This is only used for copying times, which never returns an error.
		// Therefore, this should never happen
		panic(err)
	}
	return copied.(Config)
}
