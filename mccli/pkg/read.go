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

package pkg

import (
	"fmt"
	"io"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeyaml "k8s.io/apimachinery/pkg/util/yaml"
)

// KubeKind holds a Kubernetes object with metadata
type KubeKind struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              map[string]interface{} `json:"spec"`
}

// ReadKubernetesYaml reads a file
func ReadKubernetesYaml(filename string) (*[]KubeKind, error) {
	in, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer in.Close() // nolint: errcheck

	// We store configs as a YaML stream; there may be more than one decoder.
	retval := make([]KubeKind, 0)
	yamlDecoder := kubeyaml.NewYAMLOrJSONDecoder(in, 512*1024)
	for {
		var obj KubeKind
		err := yamlDecoder.Decode(&obj)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("cannot parse: %v", err)
		}
		retval = append(retval, obj)
	}
	return &retval, nil
}
