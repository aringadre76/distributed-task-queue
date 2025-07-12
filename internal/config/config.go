package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server       ServerConfig       `yaml:"server" mapstructure:"server"`
	Database     DatabaseConfig     `yaml:"database" mapstructure:"database"`
	Redis        RedisConfig        `yaml:"redis" mapstructure:"redis"`
	Queue        QueueConfig        `yaml:"queue" mapstructure:"queue"`
	Worker       WorkerConfig       `yaml:"worker" mapstructure:"worker"`
	Logger       LoggerConfig       `yaml:"logger" mapstructure:"logger"`
	Metrics      MetricsConfig      `yaml:"metrics" mapstructure:"metrics"`
	Monitoring   MonitoringConfig   `yaml:"monitoring" mapstructure:"monitoring"`
	AutoScaling  AutoScalingConfig  `yaml:"auto_scaling" mapstructure:"auto_scaling"`
	Performance  PerformanceConfig  `yaml:"performance" mapstructure:"performance"`
	Security     SecurityConfig     `yaml:"security" mapstructure:"security"`
	Auth         AuthConfig         `yaml:"auth" mapstructure:"auth"`
	RateLimit    RateLimitConfig    `yaml:"rate_limit" mapstructure:"rate_limit"`
	Tracing      TracingConfig      `yaml:"tracing" mapstructure:"tracing"`
	Audit        AuditConfig        `yaml:"audit" mapstructure:"audit"`
}

type ServerConfig struct {
	Host         string        `yaml:"host" mapstructure:"host"`
	Port         int           `yaml:"port" mapstructure:"port"`
	ReadTimeout  time.Duration `yaml:"read_timeout" mapstructure:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout" mapstructure:"write_timeout"`
	IdleTimeout  time.Duration `yaml:"idle_timeout" mapstructure:"idle_timeout"`
}

type DatabaseConfig struct {
	Host            string        `yaml:"host" mapstructure:"host" env:"DB_HOST" envDefault:"localhost"`
	Port            int           `yaml:"port" mapstructure:"port" env:"DB_PORT" envDefault:"5432"`
	User            string        `yaml:"user" mapstructure:"user" env:"DB_USER" envDefault:"taskqueue"`
	Password        string        `yaml:"password" mapstructure:"password" env:"DB_PASSWORD" envDefault:"password"`
	DBName          string        `yaml:"db_name" mapstructure:"db_name" env:"DB_NAME" envDefault:"taskqueue"`
	SSLMode         string        `yaml:"ssl_mode" mapstructure:"ssl_mode" env:"DB_SSL_MODE" envDefault:"disable"`
	MaxOpenConns    int           `yaml:"max_open_conns" mapstructure:"max_open_conns" env:"DB_MAX_OPEN_CONNS" envDefault:"25"`
	MaxIdleConns    int           `yaml:"max_idle_conns" mapstructure:"max_idle_conns" env:"DB_MAX_IDLE_CONNS" envDefault:"5"`
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime" mapstructure:"conn_max_lifetime" env:"DB_CONN_MAX_LIFETIME" envDefault:"1h"`
	ConnMaxIdleTime time.Duration `yaml:"conn_max_idle_time" mapstructure:"conn_max_idle_time" env:"DB_CONN_MAX_IDLE_TIME" envDefault:"5m"`
	ConnectionPool  ConnectionPoolConfig `yaml:"connection_pool" mapstructure:"connection_pool"`
}

type ConnectionPoolConfig struct {
	Enabled           bool          `yaml:"enabled" env:"ENABLE_CONNECTION_POOL" envDefault:"true"`
	MinConnections    int           `yaml:"min_connections" env:"DB_MIN_CONNECTIONS" envDefault:"5"`
	MaxConnections    int           `yaml:"max_connections" env:"DB_MAX_CONNECTIONS" envDefault:"100"`
	AcquireTimeout    time.Duration `yaml:"acquire_timeout" env:"DB_ACQUIRE_TIMEOUT" envDefault:"10s"`
	HealthCheckPeriod time.Duration `yaml:"health_check_period" env:"DB_HEALTH_CHECK_PERIOD" envDefault:"1m"`
}

type RedisConfig struct {
	Host         string        `yaml:"host" mapstructure:"host"`
	Port         int           `yaml:"port" mapstructure:"port"`
	Password     string        `yaml:"password" mapstructure:"password"`
	DB           int           `yaml:"db" mapstructure:"db"`
	PoolSize     int           `yaml:"pool_size" mapstructure:"pool_size"`
	DialTimeout  time.Duration `yaml:"dial_timeout" mapstructure:"dial_timeout"`
	ReadTimeout  time.Duration `yaml:"read_timeout" mapstructure:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout" mapstructure:"write_timeout"`
}

type WorkerConfig struct {
	ID                    string        `yaml:"id" mapstructure:"id" env:"WORKER_ID"`
	PollInterval          time.Duration `yaml:"poll_interval" mapstructure:"poll_interval" env:"POLL_INTERVAL" envDefault:"5s"`
	HeartbeatInterval     time.Duration `yaml:"heartbeat_interval" mapstructure:"heartbeat_interval" env:"HEARTBEAT_INTERVAL" envDefault:"30s"`
	TaskTimeout           time.Duration `yaml:"task_timeout" mapstructure:"task_timeout" env:"TASK_TIMEOUT" envDefault:"5m"`
	MaxConcurrentTasks    int           `yaml:"max_concurrent_tasks" mapstructure:"max_concurrent_tasks" env:"MAX_CONCURRENT_TASKS" envDefault:"5"`
	TaskTypes             []string      `yaml:"task_types" mapstructure:"task_types" env:"TASK_TYPES" envDefault:"test_task,image_processing,email_sending,data_etl"`
	CircuitBreaker        CircuitBreakerConfig `yaml:"circuit_breaker" mapstructure:"circuit_breaker"`
	ErrorHandling         ErrorHandlingConfig  `yaml:"error_handling" mapstructure:"error_handling"`
	ParallelExecution     ParallelExecutionConfig `yaml:"parallel_execution" mapstructure:"parallel_execution"`
	BatchProcessing       BatchProcessingConfig   `yaml:"batch_processing" mapstructure:"batch_processing"`
}

type ParallelExecutionConfig struct {
	Enabled        bool `yaml:"enabled" mapstructure:"enabled" env:"ENABLE_PARALLEL_EXECUTION" envDefault:"true"`
	MaxConcurrency int  `yaml:"max_concurrency" mapstructure:"max_concurrency" env:"PARALLEL_CONCURRENCY" envDefault:"10"`
}

type BatchProcessingConfig struct {
	Enabled   bool          `yaml:"enabled" mapstructure:"enabled" env:"ENABLE_BATCH_PROCESSING" envDefault:"true"`
	BatchSize int           `yaml:"batch_size" mapstructure:"batch_size" env:"BATCH_SIZE" envDefault:"50"`
	FlushTime time.Duration `yaml:"flush_time" mapstructure:"flush_time" env:"BATCH_FLUSH_TIME" envDefault:"5s"`
}

type QueueConfig struct {
	RedisURL      string        `yaml:"redis_url" env:"REDIS_URL"`
	RabbitMQURL   string        `yaml:"rabbitmq_url" env:"RABBITMQ_URL"`
	QueueName     string        `yaml:"queue_name" env:"QUEUE_NAME" envDefault:"task_queue"`
	MaxRetries    int           `yaml:"max_retries" env:"MAX_RETRIES" envDefault:"3"`
	RetryDelay    time.Duration `yaml:"retry_delay" env:"RETRY_DELAY" envDefault:"30s"`
	QueueTimeout  time.Duration `yaml:"queue_timeout" env:"QUEUE_TIMEOUT" envDefault:"30s"`
	PrefetchCount int           `yaml:"prefetch_count" env:"PREFETCH_COUNT" envDefault:"10"`
	PriorityQueue PriorityQueueConfig `yaml:"priority_queue"`
	Dependencies  DependencyConfig    `yaml:"dependencies"`
}

type PriorityQueueConfig struct {
	Enabled      bool              `yaml:"enabled" env:"ENABLE_PRIORITY_QUEUE" envDefault:"true"`
	DefaultLevel string            `yaml:"default_level" env:"DEFAULT_PRIORITY" envDefault:"medium"`
	Levels       map[string]int    `yaml:"levels"`
	Weights      map[string]float64 `yaml:"weights"`
}

type DependencyConfig struct {
	Enabled           bool          `yaml:"enabled" env:"ENABLE_DEPENDENCIES" envDefault:"true"`
	MaxDependencies   int           `yaml:"max_dependencies" env:"MAX_DEPENDENCIES" envDefault:"10"`
	CircularDetection bool          `yaml:"circular_detection" env:"CIRCULAR_DETECTION" envDefault:"true"`
	DAGSupport        bool          `yaml:"dag_support" env:"DAG_SUPPORT" envDefault:"true"`
	TimeoutCheck      time.Duration `yaml:"timeout_check" env:"DEPENDENCY_TIMEOUT_CHECK" envDefault:"1m"`
}

type CircuitBreakerConfig struct {
	Enabled            bool          `mapstructure:"enabled"`
	FailureThreshold   int           `mapstructure:"failure_threshold"`
	SuccessThreshold   int           `mapstructure:"success_threshold"`
	Timeout           time.Duration `mapstructure:"timeout"`
	MonitoringPeriod  time.Duration `mapstructure:"monitoring_period"`
}

type ErrorHandlingConfig struct {
	EnableClassification bool          `mapstructure:"enable_classification"`
	DefaultMaxRetries   int           `mapstructure:"default_max_retries"`
	DefaultBaseDelay    time.Duration `mapstructure:"default_base_delay"`
	DefaultMaxDelay     time.Duration `mapstructure:"default_max_delay"`
	JitterEnabled       bool          `mapstructure:"jitter_enabled"`
}

type AutoScalingConfig struct {
	Enabled                bool          `yaml:"enabled" env:"ENABLE_AUTO_SCALING" envDefault:"true"`
	MinWorkers             int           `yaml:"min_workers" env:"MIN_WORKERS" envDefault:"2"`
	MaxWorkers             int           `yaml:"max_workers" env:"MAX_WORKERS" envDefault:"50"`
	ScaleUpThreshold       float64       `yaml:"scale_up_threshold" env:"SCALE_UP_THRESHOLD" envDefault:"80.0"`
	ScaleDownThreshold     float64       `yaml:"scale_down_threshold" env:"SCALE_DOWN_THRESHOLD" envDefault:"20.0"`
	ScaleUpCooldown        time.Duration `yaml:"scale_up_cooldown" env:"SCALE_UP_COOLDOWN" envDefault:"2m"`
	ScaleDownCooldown      time.Duration `yaml:"scale_down_cooldown" env:"SCALE_DOWN_COOLDOWN" envDefault:"5m"`
	MetricsEvaluationPeriod time.Duration `yaml:"metrics_evaluation_period" env:"METRICS_EVAL_PERIOD" envDefault:"30s"`
	QueueDepthWindow       time.Duration `yaml:"queue_depth_window" env:"QUEUE_DEPTH_WINDOW" envDefault:"5m"`
	QueueDepthWeight       float64       `yaml:"queue_depth_weight" env:"QUEUE_DEPTH_WEIGHT" envDefault:"0.6"`
	UtilizationWeight      float64       `yaml:"utilization_weight" env:"UTILIZATION_WEIGHT" envDefault:"0.4"`
}

type PerformanceConfig struct {
	Optimizations     PerformanceOptimizations `yaml:"optimizations"`
	ResourceLimits    ResourceLimitsConfig     `yaml:"resource_limits"`
	Caching          CachingConfig            `yaml:"caching"`
}

type PerformanceOptimizations struct {
	EnableConnectionPooling bool `yaml:"enable_connection_pooling" env:"ENABLE_CONNECTION_POOLING" envDefault:"true"`
	EnableBatchProcessing   bool `yaml:"enable_batch_processing" env:"ENABLE_BATCH_PROCESSING" envDefault:"true"`
	EnableParallelExecution bool `yaml:"enable_parallel_execution" env:"ENABLE_PARALLEL_EXECUTION" envDefault:"true"`
	EnableTaskPipelining    bool `yaml:"enable_task_pipelining" env:"ENABLE_TASK_PIPELINING" envDefault:"true"`
	EnableCompression       bool `yaml:"enable_compression" env:"ENABLE_COMPRESSION" envDefault:"false"`
}

type ResourceLimitsConfig struct {
	MaxMemoryUsage   int64         `yaml:"max_memory_usage" env:"MAX_MEMORY_USAGE" envDefault:"1073741824"`
	MaxCPUUsage      float64       `yaml:"max_cpu_usage" env:"MAX_CPU_USAGE" envDefault:"80.0"`
	MaxDiskUsage     int64         `yaml:"max_disk_usage" env:"MAX_DISK_USAGE" envDefault:"10737418240"`
	GCPressureLimit  float64       `yaml:"gc_pressure_limit" env:"GC_PRESSURE_LIMIT" envDefault:"70.0"`
	TaskTimeoutLimit time.Duration `yaml:"task_timeout_limit" env:"TASK_TIMEOUT_LIMIT" envDefault:"1h"`
}

type CachingConfig struct {
	Enabled        bool          `yaml:"enabled" env:"ENABLE_CACHING" envDefault:"true"`
	TTL            time.Duration `yaml:"ttl" env:"CACHE_TTL" envDefault:"1h"`
	MaxSize        int           `yaml:"max_size" env:"CACHE_MAX_SIZE" envDefault:"10000"`
	CleanupPeriod  time.Duration `yaml:"cleanup_period" env:"CACHE_CLEANUP_PERIOD" envDefault:"10m"`
	TaskResults    bool          `yaml:"task_results" env:"CACHE_TASK_RESULTS" envDefault:"true"`
	WorkerMetadata bool          `yaml:"worker_metadata" env:"CACHE_WORKER_METADATA" envDefault:"true"`
}

type LoggerConfig struct {
	Level      string `mapstructure:"level"`
	Format     string `mapstructure:"format"`
	OutputPath string `mapstructure:"output_path"`
}

type MetricsConfig struct {
	Enabled bool `mapstructure:"enabled"`
	Port    int  `mapstructure:"port"`
	Path    string `mapstructure:"path"`
}

type MonitoringConfig struct {
	Enabled bool   `yaml:"enabled" env:"ENABLE_MONITORING" envDefault:"true"`
	Port    int    `yaml:"port" env:"MONITORING_PORT" envDefault:"9090"`
	Path    string `yaml:"path" env:"MONITORING_PATH" envDefault:"/metrics"`
}

type SecurityConfig struct {
	Enabled               bool          `yaml:"enabled" mapstructure:"enabled" env:"SECURITY_ENABLED" envDefault:"true"`
	TLSEnabled            bool          `yaml:"tls_enabled" mapstructure:"tls_enabled" env:"TLS_ENABLED" envDefault:"false"`
	TLSCertFile           string        `yaml:"tls_cert_file" mapstructure:"tls_cert_file" env:"TLS_CERT_FILE"`
	TLSKeyFile            string        `yaml:"tls_key_file" mapstructure:"tls_key_file" env:"TLS_KEY_FILE"`
	CORSEnabled           bool          `yaml:"cors_enabled" mapstructure:"cors_enabled" env:"CORS_ENABLED" envDefault:"true"`
	CORSAllowedOrigins    []string      `yaml:"cors_allowed_origins" mapstructure:"cors_allowed_origins"`
	RequestTimeout        time.Duration `yaml:"request_timeout" mapstructure:"request_timeout" env:"REQUEST_TIMEOUT" envDefault:"30s"`
	MaxRequestSize        int64         `yaml:"max_request_size" mapstructure:"max_request_size" env:"MAX_REQUEST_SIZE" envDefault:"10485760"`
	EnableSecurityHeaders bool          `yaml:"enable_security_headers" mapstructure:"enable_security_headers" env:"ENABLE_SECURITY_HEADERS" envDefault:"true"`
}

type AuthConfig struct {
	Enabled       bool          `yaml:"enabled" mapstructure:"enabled" env:"AUTH_ENABLED" envDefault:"true"`
	JWTSecret     string        `yaml:"jwt_secret" mapstructure:"jwt_secret" env:"JWT_SECRET" envDefault:"your-secret-key-change-in-production"`
	TokenDuration time.Duration `yaml:"token_duration" mapstructure:"token_duration" env:"TOKEN_DURATION" envDefault:"24h"`
	RefreshEnabled bool         `yaml:"refresh_enabled" mapstructure:"refresh_enabled" env:"REFRESH_ENABLED" envDefault:"true"`
	RefreshDuration time.Duration `yaml:"refresh_duration" mapstructure:"refresh_duration" env:"REFRESH_DURATION" envDefault:"168h"`
	RequireAuth   bool          `yaml:"require_auth" mapstructure:"require_auth" env:"REQUIRE_AUTH" envDefault:"false"`
	DefaultRoles  []string      `yaml:"default_roles" mapstructure:"default_roles"`
}

type RateLimitConfig struct {
	Enabled           bool          `yaml:"enabled" mapstructure:"enabled" env:"RATE_LIMIT_ENABLED" envDefault:"true"`
	RequestsPerMinute int           `yaml:"requests_per_minute" mapstructure:"requests_per_minute" env:"RATE_LIMIT_RPM" envDefault:"100"`
	BurstLimit        int           `yaml:"burst_limit" mapstructure:"burst_limit" env:"RATE_LIMIT_BURST" envDefault:"150"`
	WindowDuration    time.Duration `yaml:"window_duration" mapstructure:"window_duration" env:"RATE_LIMIT_WINDOW" envDefault:"1m"`
	UserLimit         UserRateLimitConfig   `yaml:"user_limit" mapstructure:"user_limit"`
	TenantLimit       TenantRateLimitConfig `yaml:"tenant_limit" mapstructure:"tenant_limit"`
	IPLimit           IPRateLimitConfig     `yaml:"ip_limit" mapstructure:"ip_limit"`
}

type UserRateLimitConfig struct {
	Enabled           bool          `yaml:"enabled" mapstructure:"enabled" env:"USER_RATE_LIMIT_ENABLED" envDefault:"true"`
	RequestsPerMinute int           `yaml:"requests_per_minute" mapstructure:"requests_per_minute" env:"USER_RATE_LIMIT_RPM" envDefault:"200"`
	BurstLimit        int           `yaml:"burst_limit" mapstructure:"burst_limit" env:"USER_RATE_LIMIT_BURST" envDefault:"250"`
	WindowDuration    time.Duration `yaml:"window_duration" mapstructure:"window_duration" env:"USER_RATE_LIMIT_WINDOW" envDefault:"1m"`
}

type TenantRateLimitConfig struct {
	Enabled           bool          `yaml:"enabled" mapstructure:"enabled" env:"TENANT_RATE_LIMIT_ENABLED" envDefault:"true"`
	RequestsPerMinute int           `yaml:"requests_per_minute" mapstructure:"requests_per_minute" env:"TENANT_RATE_LIMIT_RPM" envDefault:"1000"`
	BurstLimit        int           `yaml:"burst_limit" mapstructure:"burst_limit" env:"TENANT_RATE_LIMIT_BURST" envDefault:"1500"`
	WindowDuration    time.Duration `yaml:"window_duration" mapstructure:"window_duration" env:"TENANT_RATE_LIMIT_WINDOW" envDefault:"1m"`
}

type IPRateLimitConfig struct {
	Enabled           bool          `yaml:"enabled" mapstructure:"enabled" env:"IP_RATE_LIMIT_ENABLED" envDefault:"true"`
	RequestsPerMinute int           `yaml:"requests_per_minute" mapstructure:"requests_per_minute" env:"IP_RATE_LIMIT_RPM" envDefault:"60"`
	BurstLimit        int           `yaml:"burst_limit" mapstructure:"burst_limit" env:"IP_RATE_LIMIT_BURST" envDefault:"100"`
	WindowDuration    time.Duration `yaml:"window_duration" mapstructure:"window_duration" env:"IP_RATE_LIMIT_WINDOW" envDefault:"1m"`
}

type TracingConfig struct {
	Enabled        bool   `yaml:"enabled" mapstructure:"enabled" env:"TRACING_ENABLED" envDefault:"true"`
	ServiceName    string `yaml:"service_name" mapstructure:"service_name" env:"TRACING_SERVICE_NAME" envDefault:"distributed-task-queue"`
	JaegerEndpoint string `yaml:"jaeger_endpoint" mapstructure:"jaeger_endpoint" env:"JAEGER_ENDPOINT"`
	SampleRate     float64 `yaml:"sample_rate" mapstructure:"sample_rate" env:"TRACING_SAMPLE_RATE" envDefault:"1.0"`
	CorrelationID  bool   `yaml:"correlation_id" mapstructure:"correlation_id" env:"TRACING_CORRELATION_ID" envDefault:"true"`
}

type AuditConfig struct {
	Enabled        bool     `yaml:"enabled" mapstructure:"enabled" env:"AUDIT_ENABLED" envDefault:"true"`
	LogToDatabase  bool     `yaml:"log_to_database" mapstructure:"log_to_database" env:"AUDIT_LOG_TO_DB" envDefault:"true"`
	LogToFile      bool     `yaml:"log_to_file" mapstructure:"log_to_file" env:"AUDIT_LOG_TO_FILE" envDefault:"false"`
	LogFile        string   `yaml:"log_file" mapstructure:"log_file" env:"AUDIT_LOG_FILE" envDefault:"audit.log"`
	RetentionDays  int      `yaml:"retention_days" mapstructure:"retention_days" env:"AUDIT_RETENTION_DAYS" envDefault:"90"`
	Events         []string `yaml:"events" mapstructure:"events"`
	ExcludeEvents  []string `yaml:"exclude_events" mapstructure:"exclude_events"`
}

func Load() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("./configs")
	viper.AddConfigPath(".")

	viper.AutomaticEnv()
	viper.SetEnvPrefix("DTQ")

	setDefaults()
	
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	if err := validateConfig(&config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &config, nil
}

func setDefaults() {
	viper.SetDefault("server.host", "0.0.0.0")
	viper.SetDefault("server.port", 8080)
	viper.SetDefault("server.read_timeout", "30s")
	viper.SetDefault("server.write_timeout", "30s")
	viper.SetDefault("server.idle_timeout", "120s")

	viper.SetDefault("database.host", "localhost")
	viper.SetDefault("database.port", 5432)
	viper.SetDefault("database.user", "taskqueue")
	viper.SetDefault("database.password", "password")
	viper.SetDefault("database.db_name", "taskqueue")
	viper.SetDefault("database.ssl_mode", "disable")
	viper.SetDefault("database.max_open_conns", 25)
	viper.SetDefault("database.max_idle_conns", 5)
	viper.SetDefault("database.conn_max_lifetime", "5m")

	viper.SetDefault("redis.host", "localhost")
	viper.SetDefault("redis.port", 6379)
	viper.SetDefault("redis.password", "")
	viper.SetDefault("redis.db", 0)
	viper.SetDefault("redis.pool_size", 10)
	viper.SetDefault("redis.dial_timeout", "5s")
	viper.SetDefault("redis.read_timeout", "3s")
	viper.SetDefault("redis.write_timeout", "3s")

	// Worker defaults - matching yaml struct tags
	viper.SetDefault("worker.id", "worker-1")
	viper.SetDefault("worker.max_concurrent_tasks", 5)
	viper.SetDefault("worker.task_types", []string{"test_task", "image_processing", "email_sending", "data_etl"})
	viper.SetDefault("worker.poll_interval", "1s")
	viper.SetDefault("worker.heartbeat_interval", "30s")
	viper.SetDefault("worker.task_timeout", "10m")

	// Worker sub-configs
	viper.SetDefault("worker.parallel_execution.enabled", true)
	viper.SetDefault("worker.parallel_execution.max_concurrency", 10)
	
	viper.SetDefault("worker.batch_processing.enabled", true)
	viper.SetDefault("worker.batch_processing.batch_size", 50)
	viper.SetDefault("worker.batch_processing.flush_time", "5s")

	viper.SetDefault("worker.circuit_breaker.enabled", true)
	viper.SetDefault("worker.circuit_breaker.failure_threshold", 5)
	viper.SetDefault("worker.circuit_breaker.success_threshold", 3)
	viper.SetDefault("worker.circuit_breaker.timeout", "60s")
	viper.SetDefault("worker.circuit_breaker.monitoring_period", "30s")

	viper.SetDefault("worker.error_handling.enable_classification", true)
	viper.SetDefault("worker.error_handling.default_max_retries", 3)
	viper.SetDefault("worker.error_handling.default_base_delay", "1s")
	viper.SetDefault("worker.error_handling.default_max_delay", "5m")
	viper.SetDefault("worker.error_handling.jitter_enabled", true)

	viper.SetDefault("auto_scaling.enabled", false)
	viper.SetDefault("auto_scaling.min_workers", 1)
	viper.SetDefault("auto_scaling.max_workers", 10)
	viper.SetDefault("auto_scaling.scale_up_threshold", 80.0)
	viper.SetDefault("auto_scaling.scale_down_threshold", 20.0)
	viper.SetDefault("auto_scaling.scale_up_cooldown", "5m")
	viper.SetDefault("auto_scaling.scale_down_cooldown", "10m")
	viper.SetDefault("auto_scaling.queue_depth_window", "2m")

	viper.SetDefault("logger.level", "info")
	viper.SetDefault("logger.format", "json")
	viper.SetDefault("logger.output_path", "stdout")

	viper.SetDefault("metrics.enabled", true)
	viper.SetDefault("metrics.port", 9091)
	viper.SetDefault("metrics.path", "/metrics")

	viper.SetDefault("monitoring.enabled", true)
	viper.SetDefault("monitoring.port", 9091)
	viper.SetDefault("monitoring.path", "/metrics")

	// Phase 4 defaults
	viper.SetDefault("security.enabled", true)
	viper.SetDefault("security.tls_enabled", false)
	viper.SetDefault("security.cors_enabled", true)
	viper.SetDefault("security.cors_allowed_origins", []string{"*"})
	viper.SetDefault("security.request_timeout", "30s")
	viper.SetDefault("security.max_request_size", 10485760)
	viper.SetDefault("security.enable_security_headers", true)

	viper.SetDefault("auth.enabled", true)
	viper.SetDefault("auth.jwt_secret", "your-secret-key-change-in-production")
	viper.SetDefault("auth.token_duration", "24h")
	viper.SetDefault("auth.refresh_enabled", true)
	viper.SetDefault("auth.refresh_duration", "168h")
	viper.SetDefault("auth.require_auth", false)
	viper.SetDefault("auth.default_roles", []string{"user"})

	viper.SetDefault("rate_limit.enabled", true)
	viper.SetDefault("rate_limit.requests_per_minute", 100)
	viper.SetDefault("rate_limit.burst_limit", 150)
	viper.SetDefault("rate_limit.window_duration", "1m")

	viper.SetDefault("rate_limit.user_limit.enabled", true)
	viper.SetDefault("rate_limit.user_limit.requests_per_minute", 200)
	viper.SetDefault("rate_limit.user_limit.burst_limit", 250)
	viper.SetDefault("rate_limit.user_limit.window_duration", "1m")

	viper.SetDefault("rate_limit.tenant_limit.enabled", true)
	viper.SetDefault("rate_limit.tenant_limit.requests_per_minute", 1000)
	viper.SetDefault("rate_limit.tenant_limit.burst_limit", 1500)
	viper.SetDefault("rate_limit.tenant_limit.window_duration", "1m")

	viper.SetDefault("rate_limit.ip_limit.enabled", true)
	viper.SetDefault("rate_limit.ip_limit.requests_per_minute", 60)
	viper.SetDefault("rate_limit.ip_limit.burst_limit", 100)
	viper.SetDefault("rate_limit.ip_limit.window_duration", "1m")

	viper.SetDefault("tracing.enabled", true)
	viper.SetDefault("tracing.service_name", "distributed-task-queue")
	viper.SetDefault("tracing.jaeger_endpoint", "")
	viper.SetDefault("tracing.sample_rate", 1.0)
	viper.SetDefault("tracing.correlation_id", true)

	viper.SetDefault("audit.enabled", true)
	viper.SetDefault("audit.log_to_database", true)
	viper.SetDefault("audit.log_to_file", false)
	viper.SetDefault("audit.log_file", "audit.log")
	viper.SetDefault("audit.retention_days", 90)
	viper.SetDefault("audit.events", []string{})
	viper.SetDefault("audit.exclude_events", []string{})
}

func validateConfig(config *Config) error {
	if config.Server.Port <= 0 || config.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", config.Server.Port)
	}

	if config.Database.Host == "" {
		return fmt.Errorf("database host is required")
	}

	if config.Database.Port <= 0 || config.Database.Port > 65535 {
		return fmt.Errorf("invalid database port: %d", config.Database.Port)
	}

	if config.Worker.MaxConcurrentTasks <= 0 {
		return fmt.Errorf("worker max_concurrent_tasks must be positive, got: %d", config.Worker.MaxConcurrentTasks)
	}

	if len(config.Worker.TaskTypes) == 0 {
		return fmt.Errorf("worker must support at least one task type")
	}

	if config.Metrics.Port <= 0 || config.Metrics.Port > 65535 {
		return fmt.Errorf("invalid metrics port: %d", config.Metrics.Port)
	}

	if config.Monitoring.Port <= 0 || config.Monitoring.Port > 65535 {
		return fmt.Errorf("invalid monitoring port: %d", config.Monitoring.Port)
	}

	return nil
}

func (c *Config) GetServerAddr() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}

func (c *Config) GetMonitoringAddr() string {
	return fmt.Sprintf(":%d", c.Monitoring.Port)
}

func (c *Config) GetMetricsAddr() string {
	return fmt.Sprintf(":%d", c.Metrics.Port)
} 