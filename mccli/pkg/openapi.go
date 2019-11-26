// Licensed Materials - Property of IBM
// (C) Copyright IBM Corp. 2019. All Rights Reserved.
// US Government Users Restricted Rights - Use, duplication or
// disclosure restricted by GSA ADP Schedule Contract with IBM Corp.
// Copyright 2019 IBM Corporation

package pkg

import (
	"fmt"

	"gopkg.in/yaml.v2"
)

// ToYAML prints YAML to stdout
func ToYAML(data *OpenAPI) error {
	d, err := yaml.Marshal(&data)
	if err != nil {
		return err
	}

	fmt.Printf("%s", d)
	return nil
}
