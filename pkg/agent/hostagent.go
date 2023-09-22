package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	//	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/windowseventlogreceiver"
	//	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/windowsperfcountersreceiver"

	"go.uber.org/zap"
	yaml "gopkg.in/yaml.v2"
)

// HostAgent implements Agent interface for Hosts (e.g Linux)
type HostAgent struct {
	HostConfig
	logger              *zap.Logger
	otelConfigDirectory string
	Version             string
}

// HostOptions takes in various options for HostAgent
type HostOptions func(h *HostAgent)

// WithHostAgentVersion sets the agent version
func WithHostAgentVersion(v string) HostOptions {
	return func(h *HostAgent) {
		h.Version = v
	}
}

// WithHostAgentLogger sets the logger to be used with agent logs
func WithHostAgentLogger(logger *zap.Logger) HostOptions {
	return func(h *HostAgent) {
		h.logger = logger
	}
}

// WithHostAgentOtelConfigDirectory sets the location of
// the OTEL configuration
func WithHostAgentOtelConfigDirectory(d string) HostOptions {
	return func(h *HostAgent) {
		h.otelConfigDirectory = d
	}
}

// WithHostAgentIsECSEC2 sets whether the agent is running on
// AWS ECS with EC2 infrastructure
func WithHostAgentInfraPlatform(p InfraPlatform) HostOptions {
	return func(h *HostAgent) {
		h.InfraPlatform = p
	}
}

// NewHostAgent returns new agent for Kubernetes with given options.
func NewHostAgent(cfg HostConfig, opts ...HostOptions) *HostAgent {
	var agent HostAgent
	agent.HostConfig = cfg
	for _, apply := range opts {
		apply(&agent)
	}

	if agent.logger == nil {
		agent.logger, _ = zap.NewProduction()
	}

	return &agent
}

var (
	ErrRestartStatusAPINotOK = errors.New("received error code from the server")
	ErrReceiverKeyNotFound   = errors.New("'receivers' key not found")
	ErrInvalidResponse       = errors.New("invalid response from ingestion rules api")
	ErrInvalidHostTags       = errors.New("invalid host tags, kindly check the format")
)

type configType struct {
	Docker   map[string]interface{} `json:"docker"`
	NoDocker map[string]interface{} `json:"nodocker"`
}

// DatabaseType represents the type of the database.
type DatabaseType int

const (
	PostgreSQL DatabaseType = iota
	MongoDB
	MySQL
	Redis
)

type dbConfiguration struct {
	Path string `json:"path"`
}

type apiResponseForYAML struct {
	Status        bool            `json:"status"`
	Config        configType      `json:"config"`
	PgdbConfig    dbConfiguration `json:"pgdb_config"`
	MongodbConfig dbConfiguration `json:"mongodb_config"`
	MysqlConfig   dbConfiguration `json:"mysql_config"`
	RedisConfig   dbConfiguration `json:"redis_config"`
	Message       string          `json:"message"`
}

type apiResponseForRestart struct {
	Status  bool   `json:"status"`
	Restart bool   `json:"restart"`
	Message string `json:"message"`
}

var (
	apiPathForYAML    = "api/v1/agent/ingestion-rules"
	apiPathForRestart = "api/v1/agent/restart-status"
)

const (
	yamlFile = "configyamls/nodocker/otel-config.yaml"
)

func (d DatabaseType) String() string {
	switch d {
	case PostgreSQL:
		return "postgresql"
	case MongoDB:
		return "mongodb"
	case MySQL:
		return "mysql"
	case Redis:
		return "redis"
	}
	return "unknown"
}

func (c *HostAgent) updateConfig(config map[string]interface{}, path string) (map[string]interface{}, error) {

	// Read the YAML file
	yamlData, err := os.ReadFile(path)
	if err != nil {
		return map[string]interface{}{}, err
	}

	// Unmarshal the YAML data into a temporary map[string]interface{}
	tempMap := make(map[string]interface{})
	err = yaml.Unmarshal(yamlData, &tempMap)
	if err != nil {
		return map[string]interface{}{}, err
	}

	// Add the temporary map to the existing "receiver" key
	receiverData, ok := config["receivers"].(map[string]interface{})
	if !ok {
		return map[string]interface{}{}, ErrReceiverKeyNotFound
	}

	for key, value := range tempMap {
		mapValue, mapValueOk := value.(map[interface{}]interface{})
		if mapValueOk {
			oldValue, oldValueOk := receiverData[key]
			if oldValueOk {
				oldMapValue, oldMapValueOk := oldValue.(map[string]interface{})
				if oldMapValueOk {
					for k, v := range mapValue {
						strKey, keyOk := k.(string)
						if keyOk {
							oldMapValue[strKey] = v
						} else {
							c.logger.Info("invalid key type", zap.Any("key type", k))
						}
					}
					receiverData[key] = oldMapValue
				}
			}
		}
	}

	return config, nil
}

func (c *HostAgent) updateYAML(configType, yamlPath string) error {
	// _, apiURLForYAML := checkForConfigURLOverrides()

	hostname := getHostname()

	// Call Webhook
	u, err := url.Parse(c.APIURLForConfigCheck)
	if err != nil {
		return err
	}

	baseURL := u.JoinPath(apiPathForYAML).JoinPath(c.APIKey)
	params := url.Values{}
	params.Add("config", configType)
	params.Add("platform", runtime.GOOS)
	params.Add("host_id", hostname)
	params.Add("host_tags", c.HostTags)
	params.Add("agent_version", c.Version)
	params.Add("infra_platform", fmt.Sprint(c.InfraPlatform))
	// Add Query Parameters to the URL
	baseURL.RawQuery = params.Encode() // Escape Query Parameters

	resp, err := http.Get(baseURL.String())
	if err != nil {
		c.logger.Error("failed to call get configuration api", zap.Error(err))
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.logger.Error("failed to call get configuration api", zap.Int("statuscode", resp.StatusCode))
		return ErrRestartStatusAPINotOK
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.logger.Error("failed to reas response body", zap.Error(err))
		return err
	}

	// Unmarshal JSON response into ApiResponse struct
	var apiResponse apiResponseForYAML
	if err := json.Unmarshal(body, &apiResponse); err != nil {
		c.logger.Error("failed to unmarshal api response", zap.Error(err))
		return err
	}

	// Verify API Response
	if !apiResponse.Status {
		c.logger.Error("failure status from api response for ingestion rules", zap.Bool("status", apiResponse.Status))
		return ErrInvalidResponse
	}

	var apiYAMLConfig map[string]interface{}
	if len(apiResponse.Config.Docker) == 0 && len(apiResponse.Config.NoDocker) == 0 {
		c.logger.Error("failed to get valid response",
			zap.Int("config docker len", len(apiResponse.Config.Docker)),
			zap.Int("config no docker len", len(apiResponse.Config.NoDocker)))
		return ErrInvalidResponse
	}

	apiYAMLConfig = apiResponse.Config.NoDocker
	if configType == "docker" {
		apiYAMLConfig = apiResponse.Config.Docker
	}

	dbConfigs := map[DatabaseType]dbConfiguration{
		PostgreSQL: apiResponse.PgdbConfig,
		MongoDB:    apiResponse.MongodbConfig,
		MySQL:      apiResponse.MysqlConfig,
		Redis:      apiResponse.RedisConfig,
	}

	for dbType, dbConfig := range dbConfigs {
		if c.checkDBConfigValidity(dbType, dbConfig.Path) {
			apiYAMLConfig, err = c.updateConfig(apiYAMLConfig, dbConfig.Path)
			if err != nil {
				return err
			}
		}
	}

	// Adding docker_stats settings in the config, if docker endpoint is valid
	dockerSocketPath := strings.Split(c.DockerEndpoint, "//")

	if len(dockerSocketPath) == 2 && isSocketFn(dockerSocketPath[1]) {

		if rec, o := apiYAMLConfig["receivers"].(map[string]interface{}); o {
			rec["docker_stats"] = map[string]interface{}{
				"collection_interval": "5s",
				"endpoint":            c.DockerEndpoint,
				"timeout":             "20s",
				"api_version":         "1.24",
			}
			apiYAMLConfig["receivers"] = rec

		} else {
			c.logger.Error("failed to parse receivers config")
		}

		if service, o := apiYAMLConfig["service"].(map[string]interface{}); o {
			if pipelines, o := service["pipelines"].(map[string]interface{}); o {
				pipelines["metrics/docker_stats"] = map[string]interface{}{
					"exporters":  []string{"otlp/2"},
					"processors": []string{"resourcedetection", "resource", "batch"},
					"receivers":  []string{"docker_stats"},
				}
				service["pipelines"] = pipelines
			} else {
				c.logger.Error("failed to parse pipelines config")
			}
			apiYAMLConfig["service"] = service
		} else {
			c.logger.Error("failed to parse services config")
		}

	}

	apiYAMLBytes, err := yaml.Marshal(apiYAMLConfig)
	if err != nil {
		c.logger.Error("failed to marshal api data", zap.Error(err))
		return err
	}

	if err := os.WriteFile(yamlPath, apiYAMLBytes, 0644); err != nil {
		c.logger.Error("failed to write new configuration data to file", zap.Error(err))
		return err
	}

	return nil
}

// GetUpdatedYAMLPath gets the correct otel configuration file
func (c *HostAgent) GetUpdatedYAMLPath() (string, error) {
	configType := "nodocker"
	yamlPath := yamlFile

	absYamlPath := filepath.Join(c.otelConfigDirectory, yamlPath)
	if err := c.updateYAML(configType, absYamlPath); err != nil {
		return "", err
	}

	return absYamlPath, nil
}

func (c *HostAgent) checkDBConfigValidity(dbType DatabaseType, configPath string) bool {
	if configPath != "" {
		// Check if the file exists
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			c.logger.Warn(fmt.Sprintf("%v config file not found", dbType), zap.String("path", configPath))
			return false
		}

		return true
	}
	return false
}

func restartHostAgent() error {
	//GetUpdatedYAMLPath()
	cmd := exec.Command("kill", "-SIGHUP", fmt.Sprintf("%d", os.Getpid()))
	err := cmd.Run()
	if err != nil {
		return err
	}
	return nil
}

func (c *HostAgent) callRestartStatusAPI() error {

	// fmt.Println("Starting recursive restart check......")
	// apiURLForRestart, _ := checkForConfigURLOverrides()
	hostname := getHostname()
	u, err := url.Parse(c.APIURLForConfigCheck)
	if err != nil {
		return err
	}

	baseURL := u.JoinPath(apiPathForRestart)
	baseURL = baseURL.JoinPath(c.APIKey)
	params := url.Values{}
	params.Add("host_id", hostname)
	params.Add("platform", runtime.GOOS)
	params.Add("agent_version", c.Version)
	params.Add("infra_platform", fmt.Sprint(c.InfraPlatform))

	// Add Query Parameters to the URL
	baseURL.RawQuery = params.Encode() // Escape Query Parameters

	resp, err := http.Get(baseURL.String())
	if err != nil {
		c.logger.Error("failed to call Restart-API", zap.String("url", baseURL.String()), zap.Error(err))
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.logger.Error("failed to call Restart-API", zap.Int("code", resp.StatusCode))
		return ErrRestartStatusAPINotOK
	}

	var apiResponse apiResponseForRestart
	if err := json.NewDecoder(resp.Body).Decode(&apiResponse); err != nil {
		c.logger.Error("failed unmarshal Restart-API response", zap.Error(err))
		return err
	}

	if apiResponse.Restart {
		c.logger.Info("restarting mw-agent")
		if err := restartHostAgent(); err != nil {
			c.logger.Error("error restarting mw-agent", zap.Error(err))
			return err
		}
	}

	return err
}

// ListenForConfigChanges listens for configuration changes for the
// agent on the Middleware backend and restarts the agent if configuration
// has changed.
func (c *HostAgent) ListenForConfigChanges(ctx context.Context) error {

	restartInterval, err := time.ParseDuration(c.ConfigCheckInterval)
	if err != nil {
		return err
	}

	ticker := time.NewTicker(restartInterval)

	go func() {
		for {
			c.logger.Info("check for config changes after", zap.Duration("restartInterval", restartInterval))
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				err = c.callRestartStatusAPI()
				if err != nil {
					c.logger.Info("error restarting agent on config change",
						zap.Error(err))
				}
			}
		}
	}()

	return nil
}

func (c *HostAgent) HasValidTags() bool {
	if c.HostTags == "" {
		return true
	}
	pairs := strings.Split(c.HostTags, ",")
	for _, pair := range pairs {
		keyValue := strings.Split(pair, ":")
		if len(keyValue) != 2 {
			return false
		}
	}
	return true
}
