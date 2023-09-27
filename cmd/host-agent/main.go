package main

import (
	"context"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"checkagent"

	"github.com/middleware-labs/mw-agent/pkg/agent"
	"github.com/prometheus/common/version"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/kardianos/service"
	cli "github.com/urfave/cli/v2"
	"github.com/urfave/cli/v2/altsrc"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap"
	expandconverter "go.opentelemetry.io/collector/confmap/converter/expandconverter"
	"go.opentelemetry.io/collector/confmap/provider/envprovider"
	"go.opentelemetry.io/collector/confmap/provider/fileprovider"
	"go.opentelemetry.io/collector/confmap/provider/yamlprovider"
	"go.opentelemetry.io/collector/otelcol"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var isSocketFn = isSocket

func isSocket(path string) bool {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return false
	}
	return fileInfo.Mode().Type() == fs.ModeSocket
}

var agentVersion = "0.0.1"

type program struct {
	logger    *zap.Logger
	collector *otelcol.Collector
	args      []string
}

// Service interface for kardianos/service package to run
// on Linux, Windows, MacOS & BSD
func (p *program) Start(s service.Service) error {
	// Start should not block. Do the actual work async.
	p.logger.Info("starting service", zap.Stringer("name", s))
	go p.run()
	return nil
}

func (p *program) Stop(s service.Service) error {
	// Stop should not block. Return with a few seconds.
	p.logger.Info("stopping service", zap.Stringer("name", s))
	return nil
}

func (p *program) run() {
	if err := p.collector.Run(context.Background()); err != nil {
		p.logger.Error("collector server run finished with error", zap.Error(err))
	}
}

func getFlags(cfg *agent.HostConfig) []cli.Flag {
	return []cli.Flag{
		altsrc.NewStringFlag(&cli.StringFlag{
			Name:        "api-key",
			EnvVars:     []string{"MW_API_KEY"},
			Usage:       "Middleware API key for your account.",
			Destination: &cfg.APIKey,
		}),
		altsrc.NewStringFlag(&cli.StringFlag{
			Name:        "target",
			EnvVars:     []string{"MW_TARGET"},
			Usage:       "Middleware target for your account.",
			Destination: &cfg.Target,
		}),
		altsrc.NewBoolFlag(&cli.BoolFlag{
			Name:        "enable-synthetic-monitoring",
			EnvVars:     []string{"MW_ENABLE_SYNTHETIC_MONITORING"},
			Destination: &cfg.EnableSyntheticMonitoring,
		}),
		altsrc.NewStringFlag(&cli.StringFlag{
			Name:    "config-check-interval",
			EnvVars: []string{"MW_CONFIG_CHECK_INTERVAL"},
			Usage: "Duration string to periodically check for configuration updates." +
				"Setting the value to 0 disables this feature.",
			Destination: &cfg.ConfigCheckInterval,
			DefaultText: "60s",
			Value:       "60s",
		}),
		altsrc.NewStringFlag(&cli.StringFlag{
			Name:        "docker-endpoint",
			EnvVars:     []string{"MW_DOCKER_ENDPOINT"},
			Usage:       "Set the endpoint for Docker socket if different from default",
			Destination: &cfg.DockerEndpoint,
			DefaultText: "unix:///var/run/docker.sock",
			Value:       "unix:///var/run/docker.sock",
		}),
		altsrc.NewStringFlag(&cli.StringFlag{
			Name:        "api-url-for-config-check",
			EnvVars:     []string{"MW_API_URL_FOR_CONFIG_CHECK"},
			Destination: &cfg.APIURLForConfigCheck,
			DefaultText: "https://app.middleware.io",
			Value:       "https://app.middleware.io",
			Hidden:      true,
		}),
		altsrc.NewStringFlag(&cli.StringFlag{
			Name:        "host-tags",
			Usage:       "Tags for this host",
			EnvVars:     []string{"MW_HOST_TAGS"},
			Destination: &cfg.HostTags,
		}),
		altsrc.NewStringFlag(&cli.StringFlag{
			Name:        "logfile",
			Usage:       "Log file to store Middleware agent logs",
			EnvVars:     []string{"MW_LOGFILE"},
			Destination: &cfg.Logfile,
			DefaultText: "",
			Value:       "",
		}),

		altsrc.NewIntFlag(&cli.IntFlag{
			Name:        "logfile-size",
			Usage:       "Log file size to store Middleware agent logs. This flag only applifes if logfile flag is specified",
			EnvVars:     []string{"MW_LOGFILE_SIZE"},
			Destination: &cfg.LogfileSize,
			Value:       1, // default value is 1MB
		}),

		&cli.StringFlag{
			Name:    "config-file",
			EnvVars: []string{"MW_CONFIG_FILE"},
			Usage:   "Location of the configuration file for this agent.",
			DefaultText: func() string {
				switch runtime.GOOS {
				case "linux":
					return filepath.Join("/etc", "mw-agent", "mw-agent.yaml")
				}

				return ""
			}(),
		},
	}
}

func main() {
	var cfg agent.HostConfig
	flags := getFlags(&cfg)
	zapEncoderCfg := zapcore.EncoderConfig{
		MessageKey: "message",

		LevelKey:    "level",
		EncodeLevel: zapcore.CapitalLevelEncoder,

		TimeKey:    "time",
		EncodeTime: zapcore.ISO8601TimeEncoder,

		CallerKey:    "caller",
		EncodeCaller: zapcore.ShortCallerEncoder,
	}
	zapCfg := zap.NewProductionConfig()
	zapCfg.EncoderConfig = zapEncoderCfg
	logger, _ := zapCfg.Build()
	defer func() {
		_ = logger.Sync()
	}()

	app := &cli.App{
		Name:  "mw-agent",
		Usage: "Middleware host agent",
		Commands: []*cli.Command{
			{
				Name:   "start",
				Usage:  "Start Middleware host agent",
				Flags:  flags,
				Before: altsrc.InitInputSourceWithContext(flags, altsrc.NewYamlSourceFromFlagFunc("config-file")),
				Action: func(c *cli.Context) error {
					if cfg.Logfile != "" {
						logger.Info("redirecting logs to logfile", zap.String("logfile", cfg.Logfile))
						// logfile specified. Update logger to write logs to the
						// give logfile
						w := zapcore.AddSync(&lumberjack.Logger{
							Filename:   cfg.Logfile,
							MaxSize:    cfg.LogfileSize, // megabytes
							MaxBackups: 1,
							MaxAge:     7, // days
						})
						core := zapcore.NewCore(
							zapcore.NewJSONEncoder(zapEncoderCfg),
							w,
							zap.InfoLevel,
						)

						logger = zap.New(core)
					}

					infraPlatform := agent.InfraPlatformInstance
					awsEnv := os.Getenv("AWS_EXECUTION_ENV")
					if awsEnv == "AWS_ECS_EC2" {
						infraPlatform = agent.InfraPlatformECSEC2
					}

					execPath, err := os.Executable()
					if err != nil {
						logger.Info("error getting executable path", zap.Error(err))
						return err
					}

					logger.Info("starting host agent", zap.String("agent location", execPath))
					installDir := filepath.Dir(execPath)

					logger.Info("host agent config", zap.Stringer("config", cfg),
						zap.String("version", agentVersion),
						zap.Stringer("infra-platform", infraPlatform))

					hostAgent := agent.NewHostAgent(
						cfg,
						agent.WithHostAgentLogger(logger),
						agent.WithHostAgentVersion(agentVersion),
						agent.WithHostAgentOtelConfigDirectory(installDir),
						agent.WithHostAgentInfraPlatform(infraPlatform),
					)

					ctx, cancel := context.WithCancel(c.Context)
					defer cancel()

					// Listen to the config changes provided by Middleware API
					if cfg.ConfigCheckInterval != "0" {
						err = hostAgent.ListenForConfigChanges(ctx)
						if err != nil {
							logger.Info("error for listening for config changes", zap.Error(err))
							return err
						}
					}

					if cfg.EnableSyntheticMonitoring {
						// TODO checkagent.Start should take context
						go checkagent.Start()
					}

					u, err := url.Parse(cfg.Target)
					if err != nil {
						return err
					}

					target := u.String()
					if u.Port() == "" {
						target += ":443"
					}

					// Set MW_TARGET & MW_API_KEY so that envprovider can fill those in the otel config files
					os.Setenv("MW_TARGET", target)
					os.Setenv("MW_API_KEY", cfg.APIKey)

					// TODO: check if on Windows, socket scheme is different than "unix"
					os.Setenv("MW_DOCKER_ENDPOINT", cfg.DockerEndpoint)

					// Setting MW_HOST_TAGS so that envprovider can fill those in the otel config files
					os.Setenv("MW_HOST_TAGS", cfg.HostTags)

					// Checking if host agent has valid tags
					if !hostAgent.HasValidTags() {
						return agent.ErrInvalidHostTags
					}

					// yamlPath, err := hostAgent.GetUpdatedYAMLPath()
					// if err != nil {
					// 	logger.Error("error getting config file path", zap.Error(err))
					// 	return err
					// }

					configType := "all"
					dockerSocketPath := strings.Split(cfg.DockerEndpoint, "//")
					if len(dockerSocketPath) != 2 || !isSocketFn(dockerSocketPath[1]) {
						configType = "nodocker"
					}

					yamlPath := filepath.Join("/etc", "mw-agent", "configyamls", configType, "otel-config.yaml")
					logger.Info("yaml path loaded", zap.String("path", yamlPath))

					configProvider, err := otelcol.NewConfigProvider(otelcol.ConfigProviderSettings{
						ResolverSettings: confmap.ResolverSettings{
							Providers: map[string]confmap.Provider{
								"file": fileprovider.New(),
								"yaml": yamlprovider.New(),
								"env":  envprovider.New(),
							},
							Converters: []confmap.Converter{
								expandconverter.New(),
								//overwritepropertiesconverter.New(getSetFlag()),
							},
							URIs: []string{yamlPath},
						},
					})
					if err != nil {
						logger.Error("config provider error", zap.Error(err))
						return err
					}

					factories, err := hostAgent.GetFactories(ctx)
					if err != nil {
						logger.Error("failed to get factories", zap.Error(err))
						return err
					}

					settings := otelcol.CollectorSettings{
						DisableGracefulShutdown: true,
						LoggingOptions:          []zap.Option{
							// zap.Development(),
							// zap.IncreaseLevel(zap.DebugLevel),
						},

						BuildInfo: component.BuildInfo{
							Command:     "mw-otelcontribcol",
							Description: "Middleware OpenTelemetry Collector Contrib",
							Version:     version.Version,
						},

						Factories:      factories,
						ConfigProvider: configProvider,
					}

					collector, _ := otelcol.NewCollector(settings)
					svcConfig := &service.Config{
						Name:        "MiddlewareHostAgent",
						DisplayName: "Middleware Host Agent",
						Description: "Middleware Host Agent for collecting observability signals.",
					}

					prg := &program{
						logger:    logger,
						collector: collector,
						args:      os.Args,
					}

					s, err := service.New(prg, svcConfig)
					if err != nil {
						logger.Fatal("could not create OS service", zap.Error(err))
					}

					err = s.Run()
					if err != nil {
						logger.Error("error after running the service", zap.Error(err))
					}
					return nil
				},
			},
			{
				Name:  "version",
				Usage: "Returns the current agent version",
				Action: func(c *cli.Context) error {
					fmt.Println("Middleware Agent Version", agentVersion)
					return nil
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		logger.Fatal("could not run application", zap.Error(err))
	}
}
