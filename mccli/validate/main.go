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

package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/istio-ecosystem/emcee/mccli/pkg"
)

func main() {
	var filename string
	flag.StringVar(&filename, "filename", "", "Filename")
	flag.Parse()

	if filename == "" {
		fmt.Printf("usage: validate --filename <filename>\n")
		os.Exit(1)
	}

	err := pkg.ValidateFile(filename)
	if err != nil {
		log.Fatalf("invalid: %v", err)
	}

	fmt.Printf("File is valid\n")
}
