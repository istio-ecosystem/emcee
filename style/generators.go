// Licensed Materials - Property of IBM
// (C) Copyright IBM Corp. 2019. All Rights Reserved.

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

package style

import (
	"context"

	mmv1 "github.ibm.com/istio-research/mc2019/api/v1"
)

const (
	// ProjectID is the one-word name of this project, to be used for labels etc.
	ProjectID = "emcee"
)

// MeshFedConfig creates/destroys the underlying mesh objects that implement a mmv1.MeshFedConfig
type MeshFedConfig interface {
	EffectMeshFedConfig(ctx context.Context, mfc *mmv1.MeshFedConfig) error
	RemoveMeshFedConfig(ctx context.Context, mfc *mmv1.MeshFedConfig) error
}

// ServiceBinder creates/destroys the underlying mesh objects that implement a mmv1.ServiceBinding
type ServiceBinder interface {
	EffectServiceBinding(ctx context.Context, sb *mmv1.ServiceBinding, mfc *mmv1.MeshFedConfig) error
	RemoveServiceBinding(ctx context.Context, sb *mmv1.ServiceBinding, mfc *mmv1.MeshFedConfig) error
}

// ServiceExposer creates/destroys the underlying mesh objects that implement a mmv1.ServiceExposition
type ServiceExposer interface {
	EffectServiceExposure(ctx context.Context, se *mmv1.ServiceExposition, mfc *mmv1.MeshFedConfig) error
	RemoveServiceExposure(ctx context.Context, se *mmv1.ServiceExposition, mfc *mmv1.MeshFedConfig) error
}
