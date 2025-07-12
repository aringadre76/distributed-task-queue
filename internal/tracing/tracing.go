package tracing

import (
	"context"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

const (
	CorrelationIDHeader = "X-Correlation-ID"
	TraceIDHeader       = "X-Trace-ID"
	SpanIDHeader        = "X-Span-ID"
	CorrelationIDKey    = "correlation_id"
	TraceIDKey          = "trace_id"
	SpanIDKey           = "span_id"
)

type TracingManager struct {
	tracer trace.Tracer
	logger *zap.Logger
}

func NewTracingManager(serviceName, jaegerEndpoint string, logger *zap.Logger) (*TracingManager, error) {
	var tp *sdktrace.TracerProvider
	var err error

	if jaegerEndpoint != "" {
		tp, err = initJaegerTracer(serviceName, jaegerEndpoint)
		if err != nil {
			logger.Error("Failed to initialize Jaeger tracer", zap.Error(err))
			tp = initNoOpTracer(serviceName)
		}
	} else {
		tp = initNoOpTracer(serviceName)
	}

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	tracer := otel.Tracer(serviceName)

	return &TracingManager{
		tracer: tracer,
		logger: logger,
	}, nil
}

func initJaegerTracer(serviceName, jaegerEndpoint string) (*sdktrace.TracerProvider, error) {
	exp, err := jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(jaegerEndpoint)))
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion("1.0.0"),
		)),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	return tp, nil
}

func initNoOpTracer(serviceName string) *sdktrace.TracerProvider {
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion("1.0.0"),
		)),
		sdktrace.WithSampler(sdktrace.NeverSample()),
	)

	return tp
}

func (tm *TracingManager) TracingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		correlationID := c.GetHeader(CorrelationIDHeader)
		if correlationID == "" {
			correlationID = uuid.New().String()
		}

		ctx := context.WithValue(c.Request.Context(), CorrelationIDKey, correlationID)

		spanName := fmt.Sprintf("%s %s", c.Request.Method, c.FullPath())
		ctx, span := tm.tracer.Start(ctx, spanName)
		defer span.End()

		span.SetAttributes(
			attribute.String("http.method", c.Request.Method),
			attribute.String("http.url", c.Request.URL.String()),
			attribute.String("http.route", c.FullPath()),
			attribute.String("http.user_agent", c.Request.UserAgent()),
			attribute.String("http.remote_addr", c.ClientIP()),
			attribute.String("correlation_id", correlationID),
		)

		traceID := span.SpanContext().TraceID().String()
		spanID := span.SpanContext().SpanID().String()

		c.Set(CorrelationIDKey, correlationID)
		c.Set(TraceIDKey, traceID)
		c.Set(SpanIDKey, spanID)

		c.Header(CorrelationIDHeader, correlationID)
		c.Header(TraceIDHeader, traceID)
		c.Header(SpanIDHeader, spanID)

		c.Request = c.Request.WithContext(ctx)

		start := time.Now()
		c.Next()
		duration := time.Since(start)

		span.SetAttributes(
			attribute.Int("http.status_code", c.Writer.Status()),
			attribute.Int64("http.response_size", int64(c.Writer.Size())),
			attribute.String("http.response_time", duration.String()),
		)

		if c.Writer.Status() >= 400 {
			span.SetAttributes(attribute.Bool("error", true))
		}

		tm.logger.Info("HTTP Request",
			zap.String("correlation_id", correlationID),
			zap.String("trace_id", traceID),
			zap.String("span_id", spanID),
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("duration", duration),
			zap.String("ip", c.ClientIP()),
		)
	}
}

func (tm *TracingManager) StartSpan(ctx context.Context, name string, attributes ...attribute.KeyValue) (context.Context, trace.Span) {
	ctx, span := tm.tracer.Start(ctx, name)
	
	if correlationID := GetCorrelationID(ctx); correlationID != "" {
		span.SetAttributes(attribute.String("correlation_id", correlationID))
	}
	
	if len(attributes) > 0 {
		span.SetAttributes(attributes...)
	}
	
	return ctx, span
}

func (tm *TracingManager) RecordError(span trace.Span, err error) {
	if err != nil {
		span.SetAttributes(
			attribute.Bool("error", true),
			attribute.String("error.message", err.Error()),
		)
		span.RecordError(err)
	}
}

func (tm *TracingManager) AddEvent(span trace.Span, name string, attributes ...attribute.KeyValue) {
	span.AddEvent(name, trace.WithAttributes(attributes...))
}

func GetCorrelationID(ctx context.Context) string {
	if correlationID, ok := ctx.Value(CorrelationIDKey).(string); ok {
		return correlationID
	}
	return ""
}

func GetTraceID(ctx context.Context) string {
	if traceID, ok := ctx.Value(TraceIDKey).(string); ok {
		return traceID
	}
	return ""
}

func GetSpanID(ctx context.Context) string {
	if spanID, ok := ctx.Value(SpanIDKey).(string); ok {
		return spanID
	}
	return ""
}

func GetCorrelationIDFromGin(c *gin.Context) string {
	if correlationID, exists := c.Get(CorrelationIDKey); exists {
		if id, ok := correlationID.(string); ok {
			return id
		}
	}
	return ""
}

func GetTraceIDFromGin(c *gin.Context) string {
	if traceID, exists := c.Get(TraceIDKey); exists {
		if id, ok := traceID.(string); ok {
			return id
		}
	}
	return ""
}

func GetSpanIDFromGin(c *gin.Context) string {
	if spanID, exists := c.Get(SpanIDKey); exists {
		if id, ok := spanID.(string); ok {
			return id
		}
	}
	return ""
}

func WithCorrelationID(ctx context.Context, correlationID string) context.Context {
	return context.WithValue(ctx, CorrelationIDKey, correlationID)
}

func WithTraceInfo(ctx context.Context, correlationID, traceID, spanID string) context.Context {
	ctx = context.WithValue(ctx, CorrelationIDKey, correlationID)
	ctx = context.WithValue(ctx, TraceIDKey, traceID)
	ctx = context.WithValue(ctx, SpanIDKey, spanID)
	return ctx
}

type TracedLogger struct {
	logger *zap.Logger
}

func NewTracedLogger(logger *zap.Logger) *TracedLogger {
	return &TracedLogger{logger: logger}
}

func (tl *TracedLogger) WithContext(ctx context.Context) *zap.Logger {
	fields := []zap.Field{}
	
	if correlationID := GetCorrelationID(ctx); correlationID != "" {
		fields = append(fields, zap.String("correlation_id", correlationID))
	}
	
	if traceID := GetTraceID(ctx); traceID != "" {
		fields = append(fields, zap.String("trace_id", traceID))
	}
	
	if spanID := GetSpanID(ctx); spanID != "" {
		fields = append(fields, zap.String("span_id", spanID))
	}
	
	return tl.logger.With(fields...)
}

func (tl *TracedLogger) WithGinContext(c *gin.Context) *zap.Logger {
	fields := []zap.Field{}
	
	if correlationID := GetCorrelationIDFromGin(c); correlationID != "" {
		fields = append(fields, zap.String("correlation_id", correlationID))
	}
	
	if traceID := GetTraceIDFromGin(c); traceID != "" {
		fields = append(fields, zap.String("trace_id", traceID))
	}
	
	if spanID := GetSpanIDFromGin(c); spanID != "" {
		fields = append(fields, zap.String("span_id", spanID))
	}
	
	return tl.logger.With(fields...)
} 