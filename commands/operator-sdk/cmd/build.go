// Copyright 2018 The Operator-SDK Authors
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

package cmd

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"

	"github.com/operator-framework/operator-sdk/commands/operator-sdk/cmd/cmdutil"
	cmdError "github.com/operator-framework/operator-sdk/commands/operator-sdk/error"
	"github.com/operator-framework/operator-sdk/pkg/generator"

	"github.com/spf13/cobra"
)

func NewBuildCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "build <image> <path to template>",
		Short: "Compiles code and builds artifacts",
		Long: `The operator-sdk build command compiles the code, builds the executables,
and generates Kubernetes manifests.

<image> is the container image to be built, e.g. "quay.io/example/operator:v0.0.1".
This image will be automatically set in the deployment manifests.

<path to template> is the path to a an (optional) custom template for the deploy/operator.yaml file

After build completes, the image would be built locally in docker. Then it needs to
be pushed to remote registry.
For example:
	$ operator-sdk build quay.io/example/operator:v0.0.1
	$ docker push quay.io/example/operator:v0.0.1
`,
		Run: buildFunc,
	}
}

const (
	build       = "./tmp/build/build.sh"
	dockerBuild = "./tmp/build/docker_build.sh"
	configYaml  = "./config/config.yaml"
)

func buildFunc(cmd *cobra.Command, args []string) {
	if len(args) != 1 || len(args) != 2 {
		cmdError.ExitWithError(cmdError.ExitBadArgs, fmt.Errorf("build command needs at least 1 argument."))
	}

	bcmd := exec.Command(build)
	o, err := bcmd.CombinedOutput()
	if err != nil {
		cmdError.ExitWithError(cmdError.ExitError, fmt.Errorf("failed to build: (%v)", string(o)))
	}
	fmt.Fprintln(os.Stdout, string(o))

	image := args[0]
	dbcmd := exec.Command(dockerBuild)
	dbcmd.Env = append(os.Environ(), fmt.Sprintf("IMAGE=%v", image))
	o, err = dbcmd.CombinedOutput()
	if err != nil {
		cmdError.ExitWithError(cmdError.ExitError, fmt.Errorf("failed to output build image %v: (%v)", image, string(o)))
	}
	fmt.Fprintln(os.Stdout, string(o))

	c := cmdutil.GetConfig()
	if len(args) == 1 {
		if renderErr := generator.RenderOperatorYaml(c, image); renderErr != nil {
			cmdError.ExitWithError(cmdError.ExitError, fmt.Errorf("failed to generate deploy/operator.yaml: (%v)", renderErr))
		}
	} else if len(args) == 2 {
		templFile, fileErr := ioutil.ReadFile(args[2])
		if fileErr != nil {
			cmdError.ExitWithError(cmdError.ExitError, fmt.Errorf("failed to read template file: (%v)", fileErr))
		}

		if renderErr := generator.RenderCustomOperatorYaml(c, image, string(templFile)); renderErr != nil {
			cmdError.ExitWithError(cmdError.ExitError, fmt.Errorf("failed to generate deploy/operator.yaml: (%v)", renderErr))
		}
	}
}
