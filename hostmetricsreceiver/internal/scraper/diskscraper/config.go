// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package diskscraper // import "collector-agent/hostmetricsreceiver/internal/scraper/diskscraper"

import (
	"collector-agent/hostmetricsreceiver/coreinternal/processor/filterset"
	"collector-agent/hostmetricsreceiver/internal/scraper/diskscraper/internal/metadata"
)

// Config relating to Disk Metric Scraper.
type Config struct {
	// Metrics allows to customize scraped metrics representation.
	Metrics metadata.MetricsSettings `mapstructure:"metrics"`

	// Include specifies a filter on the devices that should be included from the generated metrics.
	// Exclude specifies a filter on the devices that should be excluded from the generated metrics.
	// If neither `include` or `exclude` are set, metrics will be generated for all devices.
	Include MatchConfig `mapstructure:"include"`
	Exclude MatchConfig `mapstructure:"exclude"`
}

type MatchConfig struct {
	filterset.Config `mapstructure:",squash"`

	Devices []string `mapstructure:"devices"`
}
