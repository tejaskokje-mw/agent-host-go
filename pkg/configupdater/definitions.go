package configupdater

import (
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"strings"
)

// Otel config components
const (
	Receivers              = "receivers"
	AWSECSContainerMetrics = "awsecscontainermetrics"
	Service                = "service"
	Pipelines              = "pipelines"
	Metrics                = "metrics"
)

var (
	ErrInvalidTarget = fmt.Errorf("invalid target")
)

// InfraPlatform defines the agent's infrastructure platform
type InfraPlatform uint16

var (
	// InfraPlatformInstance is for bare metal or VM platform
	InfraPlatformInstance InfraPlatform = 0
	// InfraPlatformKubernetes is for Kubernetes platform
	InfraPlatformKubernetes InfraPlatform = 1
	// InfraPlatformECSEC2 is for AWS ECS EC2 platform
	InfraPlatformECSEC2 InfraPlatform = 2
	// InfraPlatformECSFargate is for AWS ECS Fargate platform
	InfraPlatformECSFargate InfraPlatform = 3
	// InfraPlatformCycleIO is for Cycle.io platform
	InfraPlatformCycleIO InfraPlatform = 4
)

func (p InfraPlatform) String() string {
	switch p {
	case InfraPlatformInstance:
		return "instance"
	case InfraPlatformKubernetes:
		return "kubernetes"
	case InfraPlatformECSEC2:
		return "ecsec2"
	case InfraPlatformECSFargate:
		return "ecsfargate"
	case InfraPlatformCycleIO:
		return "cycleio"
	}
	return "unknown"
}

type configType struct {
	Docker     map[string]interface{} `json:"docker"`
	NoDocker   map[string]interface{} `json:"nodocker"`
	Deployment map[string]interface{} `json:"deployment"`
	DaemonSet  map[string]interface{} `json:"daemonset"`
}

var (
	apiPathForYAML    = "api/v1/agent/ingestion-rules"
	apiPathForRestart = "api/v1/agent/restart-status"
)

type apiResponseForYAML struct {
	Status bool       `json:"status"`
	Config configType `json:"config"`
	/*PgdbConfig          integrationConfiguration `json:"pgdb_config"`
	MongodbConfig       integrationConfiguration `json:"mongodb_config"`
	MysqlConfig         integrationConfiguration `json:"mysql_config"`
	RedisConfig         integrationConfiguration `json:"redis_config"`
	ElasticsearchConfig integrationConfiguration `json:"elasticsearch_config"`
	CassandraConfig     integrationConfiguration `json:"cassandra_config"`
	ClickhouseConfig    integrationConfiguration `json:"clickhouse_config"`*/
	Message string `json:"message"`
}

type rollout struct {
	Deployment bool `json:"deployment"`
	Daemonset  bool `json:"daemonset"`
}

type apiResponseForRestart struct {
	Status  bool    `json:"status"`
	Restart bool    `json:"restart"`
	Rollout rollout `json:"rollout"`
	Message string  `json:"message"`
}

type Client struct {
	APIURLForConfigCheck string
	APIKey               string
	Target               string
	InfraPlatform        InfraPlatform
}

type AgentFeatures struct {
	MetricCollection    bool
	LogCollection       bool
	SyntheticMonitoring bool
}

type BaseConfig struct {
	APIKey                       string
	Target                       string
	EnableSyntheticMonitoring    bool
	ConfigCheckInterval          string
	FetchAccountOtelConfig       bool
	DockerEndpoint               string
	APIURLForConfigCheck         string
	APIURLForSyntheticMonitoring string
	GRPCPort                     string
	HTTPPort                     string
	FluentPort                   string
	InfraPlatform                InfraPlatform
	OtelConfigFile               string
	AgentFeatures                AgentFeatures
	SelfProfiling                bool
	ProfilngServerURL            string
	InternalMetricsPort          uint
}

// String() implements stringer interface for BaseConfig
func (c BaseConfig) String() string {
	var s string
	s += fmt.Sprintf("api-key: %s, ", c.APIKey)
	s += fmt.Sprintf("target: %s, ", c.Target)
	s += fmt.Sprintf("enable-synthetic-monitoring: %t, ", c.EnableSyntheticMonitoring)
	s += fmt.Sprintf("config-check-interval: %s, ", c.ConfigCheckInterval)
	s += fmt.Sprintf("docker-endpoint: %s, ", c.DockerEndpoint)
	s += fmt.Sprintf("api-url-for-config-check: %s, ", c.APIURLForConfigCheck)
	s += fmt.Sprintf("infra-platform: %s, ", c.InfraPlatform)
	s += fmt.Sprintf("agent-features: %#v, ", c.AgentFeatures)
	s += fmt.Sprintf("fluent-port: %#v, ", c.FluentPort)
	return s
}

// HostConfig stores configuration for all the host agent
type HostConfig struct {
	BaseConfig

	HostTags     string
	Logfile      string
	LogfileSize  int
	LoggingLevel string
}

// String() implements stringer interface for HostConfig
func (h HostConfig) String() string {
	s := h.BaseConfig.String()
	s += fmt.Sprintf("host-tags: %s, ", h.HostTags)
	s += fmt.Sprintf("logfile: %s, ", h.Logfile)
	s += fmt.Sprintf("logfile-size: %d", h.LogfileSize)
	return s
}

// KubeConfig stores configuration for all the host agent
type KubeConfig struct {
	BaseConfig
}

type KubeAgentConfigConfig struct {
	AgentNamespace      string
	Daemonset           string
	Deployment          string
	DaemonsetConfigMap  string
	DeploymentConfigMap string
}

// WithKubeAgentConfigVersion sets the agent version
func WithKubeAgentConfigVersion(v string) KubeAgentConfigOptions {
	return func(h *KubeAgentConfig) {
		h.Version = v
	}
}

// WithKubeAgentConfigClusterName sets the cluster name
func WithKubeAgentConfigClusterName(v string) KubeAgentConfigOptions {
	return func(k *KubeAgentConfig) {
		k.ClusterName = v
	}
}

// WithKubeAgentConfigDaemonset sets the daemonset name for the agent
func WithKubeAgentConfigDaemonset(v string) KubeAgentConfigOptions {
	return func(k *KubeAgentConfig) {
		k.Daemonset = v
	}
}

// WithKubeAgentConfigDeployment sets the deployment name for the agent
func WithKubeAgentConfigDeployment(v string) KubeAgentConfigOptions {
	return func(k *KubeAgentConfig) {
		k.Deployment = v
	}
}

// WithKubeAgentConfigAgentNamespace sets the namespace where the agent is running
func WithKubeAgentConfigAgentNamespace(v string) KubeAgentConfigOptions {
	return func(k *KubeAgentConfig) {
		k.AgentNamespace = v
	}
}

// WithKubeAgentConfigDaemonsetConfigMap sets the configmap name for the agent daemonset
func WithKubeAgentConfigDaemonsetConfigMap(v string) KubeAgentConfigOptions {
	return func(k *KubeAgentConfig) {
		k.DaemonsetConfigMap = v
	}
}

// WithKubeAgentConfigDeploymentConfigMap sets the configmap name for the agent deployment
func WithKubeAgentConfigDeploymentConfigMap(v string) KubeAgentConfigOptions {
	return func(k *KubeAgentConfig) {
		k.DeploymentConfigMap = v
	}
}

// String() implements stringer interface for KubeConfig
func (k KubeConfig) String() string {
	s := k.BaseConfig.String()
	return s
}

func isSocket(path string) bool {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return false
	}
	return fileInfo.Mode().Type() == fs.ModeSocket
}

func GetAPIURLForConfigCheck(target string) (string, error) {

	// There should at least be two "." in the URL
	parts := strings.Split(target, ".")
	if len(parts) < 3 {
		return "", ErrInvalidTarget
	}

	return strings.TrimSuffix(target, "/"), nil
}

// GetAPIURLForSyntheticMonitoring constructs the WebSocket URL for synthetic monitoring
func GetAPIURLForSyntheticMonitoring(target string) (string, error) {
	// Parse the URL
	parsedURL, err := url.Parse(target)
	if err != nil {
		return "", err
	}

	// Check if the host part of the URL contains more than one '.'
	hostParts := strings.Split(parsedURL.Hostname(), ".")
	if len(hostParts) < 3 {
		return "", ErrInvalidTarget
	}

	// Ensure no trailing slash in the path
	trimmedURL := strings.TrimSuffix(parsedURL.Host, "/")

	// Build the WebSocket URL
	webSocketURL := "wss://" + trimmedURL + "/plsrws/v2"
	return webSocketURL, nil
}
