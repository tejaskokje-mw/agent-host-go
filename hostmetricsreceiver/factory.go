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

package hostmetricsreceiver // import "collector-agent/hostmetricsreceiver"

import (
	"context"
	"fmt"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/receiver/scraperhelper"

	"collector-agent/hostmetricsreceiver/internal"
	"collector-agent/hostmetricsreceiver/internal/scraper/cpuscraper"
	"collector-agent/hostmetricsreceiver/internal/scraper/diskscraper"
	"collector-agent/hostmetricsreceiver/internal/scraper/filesystemscraper"
	"collector-agent/hostmetricsreceiver/internal/scraper/loadscraper"
	"collector-agent/hostmetricsreceiver/internal/scraper/memoryscraper"
	"collector-agent/hostmetricsreceiver/internal/scraper/networkscraper"
	"collector-agent/hostmetricsreceiver/internal/scraper/pagingscraper"
	"collector-agent/hostmetricsreceiver/internal/scraper/processesscraper"
	"collector-agent/hostmetricsreceiver/internal/scraper/processscraper"
)

// This file implements Factory for HostMetrics receiver.

const (
	// The value of "type" key in configuration.
	typeStr = "hostmetrics"
)

var (
	scraperFactories = map[string]internal.ScraperFactory{
		cpuscraper.TypeStr:        &cpuscraper.Factory{},
		diskscraper.TypeStr:       &diskscraper.Factory{},
		loadscraper.TypeStr:       &loadscraper.Factory{},
		filesystemscraper.TypeStr: &filesystemscraper.Factory{},
		memoryscraper.TypeStr:     &memoryscraper.Factory{},
		networkscraper.TypeStr:    &networkscraper.Factory{},
		pagingscraper.TypeStr:     &pagingscraper.Factory{},
		processesscraper.TypeStr:  &processesscraper.Factory{},
		processscraper.TypeStr:    &processscraper.Factory{},
	}
)

// NewFactory creates a new factory for host metrics receiver.
func NewFactory() component.ReceiverFactory {
	return component.NewReceiverFactory(
		typeStr,
		createDefaultConfig,
		component.WithMetricsReceiver(createMetricsReceiver))
}

func getScraperFactory(key string) (internal.ScraperFactory, bool) {
	if factory, ok := scraperFactories[key]; ok {
		return factory, true
	}

	return nil, false
}

// createDefaultConfig creates the default configuration for receiver.
func createDefaultConfig() config.Receiver {
	return &Config{ScraperControllerSettings: scraperhelper.NewDefaultScraperControllerSettings(typeStr)}
}

// createMetricsReceiver creates a metrics receiver based on provided config.
func createMetricsReceiver(
	ctx context.Context,
	set component.ReceiverCreateSettings,
	cfg config.Receiver,
	consumer consumer.Metrics,
) (component.MetricsReceiver, error) {
	oCfg := cfg.(*Config)

	addScraperOptions, err := createAddScraperOptions(ctx, set, oCfg, scraperFactories)
	if err != nil {
		return nil, err
	}

	return scraperhelper.NewScraperControllerReceiver(
		&oCfg.ScraperControllerSettings,
		set,
		consumer,
		addScraperOptions...,
	)
}

func createAddScraperOptions(
	ctx context.Context,
	set component.ReceiverCreateSettings,
	config *Config,
	factories map[string]internal.ScraperFactory,
) ([]scraperhelper.ScraperControllerOption, error) {
	scraperControllerOptions := make([]scraperhelper.ScraperControllerOption, 0, len(config.Scrapers))

	for key, cfg := range config.Scrapers {
		hostMetricsScraper, ok, err := createHostMetricsScraper(ctx, set, key, cfg, factories)
		if err != nil {
			return nil, fmt.Errorf("failed to create scraper for key %q: %w", key, err)
		}

		if ok {
			scraperControllerOptions = append(scraperControllerOptions, scraperhelper.AddScraper(hostMetricsScraper))
			continue
		}

		return nil, fmt.Errorf("host metrics scraper factory not found for key: %q", key)
	}

	return scraperControllerOptions, nil
}

func createHostMetricsScraper(ctx context.Context, set component.ReceiverCreateSettings, key string, cfg internal.Config, factories map[string]internal.ScraperFactory) (scraper scraperhelper.Scraper, ok bool, err error) {
	factory := factories[key]
	if factory == nil {
		ok = false
		return
	}

	ok = true
	scraper, err = factory.CreateMetricsScraper(ctx, set, cfg)
	return
}
