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

package chartutil

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/iancoleman/strcase"
	log "github.com/sirupsen/logrus"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/downloader"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/repo"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/kubebuilder/v3/pkg/config"
	"sigs.k8s.io/kubebuilder/v3/pkg/model/resource"
	"sigs.k8s.io/kubebuilder/v3/pkg/plugins/golang"
)

const (

	// HelmChartsDir is the relative directory within an SDK project where Helm
	// charts are stored.
	HelmChartsDir string = "helm-charts"

	// DefaultGroup is the Kubernetes CRD API Group used for fetched
	// charts when the --group flag is not specified
	DefaultGroup string = "charts"

	// DefaultVersion is the Kubernetes CRD API Version used for fetched
	// charts when the --version flag is not specified
	DefaultVersion string = "v1alpha1"
)

// CreateOptions is used to configure how a Helm chart is scaffolded
// for a new Helm operator project.
type CreateOptions struct {
	GVK schema.GroupVersionKind

	// Chart is a chart reference for a local or remote chart.
	Chart string

	// Repo is a URL to a custom chart repository.
	Repo string

	// Version is the version of the chart to fetch.
	Version string

	// CRDVersion is the version of the `apiextensions.k8s.io` API which will be used to generate the CRD.
	CRDVersion string

	// Domain is the domain of the project
	Domain string
}

// CreateChart creates a new helm chart based on the passed opts.
//
// It returns a scaffold.Resource that can be used by the caller to create
// other related files. opts.ResourceAPIVersion and opts.ResourceKind are
// used to create the resource and must be specified if opts.Chart is empty.
//
// If opts.Chart is not empty, opts.ResourceAPIVersion and opts.Kind can be
// left unset: opts.ResourceAPIVersion defaults to "charts.helm.k8s.io/v1alpha1"
// and opts.ResourceKind is deduced from the specified opts.Chart.
//
// CreateChart also returns the newly created chart.Chart.
//
// If opts.Chart is empty, CreateChart creates the default chart from helm's
// default template.
//
// If opts.Chart is a local file, CreateChart verifies that it is a valid helm
// chart archive and returns its chart.Chart representation.
//
// If opts.Chart is a local directory, CreateChart verifies that it is a valid
// helm chart directory and returns its chart.Chart representation.
//
// For any other value of opts.Chart, CreateChart attempts to fetch the helm chart
// from a remote repository.
//
// If opts.Repo is not specified, the following chart reference formats are supported:
//
//   - <repoName>/<chartName>: Fetch the helm chart named chartName from the helm
//                             chart repository named repoName, as specified in the
//                             $HELM_HOME/repositories/repositories.yaml file.
//
//   - <url>: Fetch the helm chart archive at the specified URL.
//
// If opts.Repo is specified, only one chart reference format is supported:
//
//   - <chartName>: Fetch the helm chart named chartName in the helm chart repository
//                  specified by opts.Repo
//
// If opts.Version is not set, CreateChart will fetch the latest available version of
// the helm chart. Otherwise, CreateChart will fetch the specified version.
// opts.Version is not used when opts.Chart itself refers to a specific version, for
// example when it is a local path or a URL.
//
// CreateChart returns an error if an error occurs creating the resource.Resource or
// loading the chart.Chart.
func CreateChart(cfg config.Config, opts CreateOptions) (r *resource.Resource, c *chart.Chart, err error) {
	tmpDir, err := ioutil.TempDir("", "osdk-helm-chart")
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			log.Errorf("Failed to remove temporary chart directory %s: %s", tmpDir, err)
		}
	}()

	// If we don't have a helm chart reference, scaffold the default chart
	// from Helm's default template. Otherwise, fetch it.
	if len(opts.Chart) == 0 {
		r, c, err = scaffoldChart(cfg, tmpDir, opts)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to scaffold default chart: %v", err)
		}
	} else {
		r, c, err = fetchChart(cfg, tmpDir, opts)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to fetch chart: %v", err)
		}
	}

	absChartPath := filepath.Join(tmpDir, c.Name())
	if err := fetchChartDependencies(absChartPath); err != nil {
		return nil, nil, fmt.Errorf("failed to fetch chart dependencies: %v", err)
	}

	// Reload chart in case dependencies changed
	c, err = loader.Load(absChartPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load chart: %v", err)
	}

	return r, c, nil
}

func scaffoldChart(cfg config.Config, destDir string, opts CreateOptions) (*resource.Resource, *chart.Chart, error) {
	chartPath, err := chartutil.Create(strings.ToLower(opts.GVK.Kind), destDir)
	if err != nil {
		return nil, nil, err
	}
	chart, err := loader.Load(chartPath)
	if err != nil {
		return nil, nil, err
	}

	return opts.NewResource(cfg), chart, nil
}

func fetchChart(cfg config.Config, destDir string, opts CreateOptions) (_ *resource.Resource, chart *chart.Chart, err error) {
	if _, err = os.Stat(opts.Chart); err == nil {
		chart, err = createChartFromDisk(destDir, opts.Chart)
	} else {
		chart, err = createChartFromRemote(destDir, opts)
	}
	if err != nil {
		return nil, nil, err
	}

	chartName := chart.Name()
	if len(opts.GVK.Group) == 0 {
		opts.GVK.Group = DefaultGroup
	}
	if len(opts.GVK.Version) == 0 {
		opts.GVK.Version = DefaultVersion
	}
	if len(opts.GVK.Kind) == 0 {
		opts.GVK.Kind = strcase.ToCamel(chartName)
	}

	return opts.NewResource(cfg), chart, nil
}

func (opts CreateOptions) NewResource(cfg config.Config) *resource.Resource {
	ro := &golang.Options{}
	ro.DoAPI = true
	ro.Namespaced = true
	ro.Domain = opts.Domain
	ro.CRDVersion = opts.CRDVersion
	ro.Group = opts.GVK.Group
	ro.Version = opts.GVK.Version
	ro.Kind = opts.GVK.Kind

	r := ro.NewResource(cfg)
	r.Domain = cfg.GetDomain()
	// remove the path since is not a Go project
	r.Path = ""
	return &r
}

func createChartFromDisk(destDir, source string) (*chart.Chart, error) {
	chart, err := loader.Load(source)
	if err != nil {
		return nil, err
	}

	// Save it into destDir.
	if err := chartutil.SaveDir(chart, destDir); err != nil {
		return nil, err
	}
	return chart, nil
}

func createChartFromRemote(destDir string, opts CreateOptions) (*chart.Chart, error) {
	settings := cli.New()
	getters := getter.All(settings)
	c := downloader.ChartDownloader{
		Out:              os.Stderr,
		Getters:          getters,
		RepositoryConfig: settings.RepositoryConfig,
		RepositoryCache:  settings.RepositoryCache,
	}

	if opts.Repo != "" {
		chartURL, err := repo.FindChartInRepoURL(opts.Repo, opts.Chart, opts.Version, "", "", "", getters)
		if err != nil {
			return nil, err
		}
		opts.Chart = chartURL
	}

	chartArchive, _, err := c.DownloadTo(opts.Chart, opts.Version, destDir)
	if err != nil {
		return nil, err
	}

	return createChartFromDisk(destDir, chartArchive)
}

func fetchChartDependencies(chartPath string) error {
	settings := cli.New()
	getters := getter.All(settings)

	out := &bytes.Buffer{}
	man := &downloader.Manager{
		Out:              out,
		ChartPath:        chartPath,
		Getters:          getters,
		RepositoryConfig: settings.RepositoryConfig,
		RepositoryCache:  settings.RepositoryCache,
	}
	if err := man.Build(); err != nil {
		fmt.Println(out.String())
		return err
	}
	return nil
}
