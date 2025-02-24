package main

import (
	"context"
	"os"

	configupdater "github.com/middleware-labs/mw-agent/pkg/configupdater"
	"github.com/urfave/cli/v2"
	"github.com/urfave/cli/v2/altsrc"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var agentVersion = "0.0.1"

func getFlags(cfg *configupdater.KubeConfig) []cli.Flag {
	return []cli.Flag{
		altsrc.NewStringFlag(&cli.StringFlag{
			Name:        "api-key",
			EnvVars:     []string{"MW_API_KEY"},
			Usage:       "Middleware API key for your account.",
			Destination: &cfg.APIKey,
		}),
		altsrc.NewStringFlag(&cli.StringFlag{
			Name:        "target",
			EnvVars:     []string{"MW_TARGET", "TARGET"},
			Usage:       "Middleware target for your account.",
			Destination: &cfg.Target,
		}),

		altsrc.NewStringFlag(&cli.StringFlag{
			Name:    "config-check-interval",
			EnvVars: []string{"MW_CONFIG_CHECK_INTERVAL"},
			Usage: "Duration string to periodically check for configuration updates." +
				"Setting the value to 0 disables this feature.",
			Destination: &cfg.ConfigCheckInterval,
			DefaultText: "60s",
		}),
		altsrc.NewStringFlag(&cli.StringFlag{
			Name:        "api-url-for-config-check",
			EnvVars:     []string{"MW_API_URL_FOR_CONFIG_CHECK"},
			Destination: &cfg.APIURLForConfigCheck,
			DefaultText: "",
			Value:       "",
			Hidden:      true,
		}),
	}
}

func main() {
	var cfg configupdater.KubeConfig
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

	mwNamespace := os.Getenv("MW_NAMESPACE")
	if mwNamespace == "" {
		mwNamespace = "mw-agent-ns"
	}

	KubeAgentConfig := configupdater.NewKubeAgentConfig(cfg,
		configupdater.WithKubeAgentConfigClusterName(os.Getenv("MW_KUBE_CLUSTER_NAME")),
		configupdater.WithKubeAgentConfigAgentNamespace(mwNamespace),
		configupdater.WithKubeAgentConfigDaemonset("mw-kube-agent"),
		configupdater.WithKubeAgentConfigDeployment("mw-kube-agent"),
		configupdater.WithKubeAgentConfigDaemonsetConfigMap("mw-daemonset-otel-config"),
		configupdater.WithKubeAgentConfigDeploymentConfigMap("mw-deployment-otel-config"),
		configupdater.WithKubeAgentConfigVersion(agentVersion),
	)

	err := KubeAgentConfig.SetClientSet()
	if err != nil {
		logger.Fatal("collector server run finished with error", zap.Error(err))
		return
	}

	app := &cli.App{
		Name:  "mw-config-updater",
		Usage: "Middleware Kubernetes Agent Configuration Updater",
		Commands: []*cli.Command{
			{
				Name:  "update",
				Usage: "Watch for configuration updates and restart the agent when a change is detected",
				Flags: flags,
				Action: func(c *cli.Context) error {
					ctx, cancel := context.WithCancel(c.Context)
					defer cancel()
					if cfg.APIURLForConfigCheck == "" {
						var err error
						cfg.APIURLForConfigCheck, err = configupdater.GetAPIURLForConfigCheck(cfg.Target)
						// could not derive api url for config check from target
						if err != nil {
							logger.Info("could not derive api url for config check from target",
								zap.String("target", cfg.Target))
							return err
						}

						logger.Info("derived api url for config check",
							zap.String("api-url-for-config-check", cfg.APIURLForConfigCheck))
					}

					if cfg.ConfigCheckInterval != "0" {
						err = KubeAgentConfig.ListenForConfigChanges(ctx)
						if err != nil {
							logger.Info("error for listening for config changes", zap.Error(err))
							return err
						}
					}
					return nil
				},
			},
			{
				Name:  "force-update-configmaps",
				Usage: "Update the configmaps as per Server settings",
				Flags: flags,
				Action: func(c *cli.Context) error {

					ctx, cancel := context.WithCancel(c.Context)
					defer func() {
						cancel()
					}()

					KubeAgentConfig.UpdateConfigMap(ctx, configupdater.Deployment)
					KubeAgentConfig.UpdateConfigMap(ctx, configupdater.DaemonSet)

					return nil

				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		logger.Fatal("could not run application", zap.Error(err))
	}
}
