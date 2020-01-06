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
	"regexp"
	"testing"
)

type testcase struct {
	filename       string
	expectedRegexp *regexp.Regexp
}

func TestValidation(t *testing.T) {
	cases := []testcase{
		{
			filename:       "test/samples/invalid-mfc-mode.yaml",
			expectedRegexp: regexp.MustCompile("Unknown Mode \"FLAT\""),
		},
		{
			filename:       "test/samples/invalid-mfc-egress.yaml",
			expectedRegexp: regexp.MustCompile("does not specify egress, but selects one"),
		},
		{
			filename:       "test/samples/invalid-expose.yaml",
			expectedRegexp: regexp.MustCompile("requires mesh_fed_config_selector"),
		},
		{
			filename:       "test/samples/invalid-bind.yaml",
			expectedRegexp: regexp.MustCompile("helloworld: invalid alias"),
		},
		{
			filename: "samples/limited-trust/limited-trust-c1.yaml",
		},
		{
			filename: "samples/limited-trust/helloworld-expose.yaml",
		},
		{
			filename: "samples/limited-trust/helloworld-binding.yaml",
		},
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("case %d %s", i, c.filename), func(t *testing.T) {
			verifyValidation(t, c)
		})
	}
}

func verifyValidation(t *testing.T, c testcase) {
	t.Helper()

	filename := "../../" + c.filename
	_, err := ReadKubernetesYaml(filename)
	if err != nil {
		t.Fatalf("Could not load test file %s", filename)
	}

	fErr := ValidateFile(filename)

	if c.expectedRegexp != nil {
		if fErr == nil {
			t.Fatalf("Wanted an exception for %v, didn't get one", c)
		}
		if !c.expectedRegexp.MatchString(fErr.Error()) {
			t.Fatalf("Exception didn't match for %v\n got %v\nwant: %v",
				c, fErr.Error(), c.expectedRegexp)

		}
	} else {
		if fErr != nil {
			t.Fatalf("Unwanted exception for %v: %v", c, fErr)
		}
	}
}
