package telemetry

import (
	"context"
	"encoding/base64"
	"errors"
	"net/url"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

func Start(ctx context.Context) (func(context.Context) error, error) {
	public, secret := os.Getenv("LANGFUSE_PUBLIC_KEY"), os.Getenv("LANGFUSE_SECRET_KEY")
	if public == "" || secret == "" {
		return func(context.Context) error { return nil }, nil
	}
	base := os.Getenv("LANGFUSE_BASE_URL")
	if base == "" {
		base = "https://cloud.langfuse.com"
	}
	endpoint, err := url.Parse(strings.TrimRight(base, "/") + "/api/public/otel/v1/traces")
	if err != nil {
		return nil, err
	}
	option := otlptracehttp.WithEndpointURL(endpoint.String())
	exporter, err := otlptracehttp.New(ctx, option, otlptracehttp.WithHeaders(map[string]string{
		"Authorization":                "Basic " + base64.StdEncoding.EncodeToString([]byte(public+":"+secret)),
		"x-langfuse-ingestion-version": "4",
	}))
	if err != nil {
		return nil, err
	}
	res, err := resource.New(ctx, resource.WithAttributes(semconv.ServiceName("docket")))
	if err != nil {
		return nil, errors.Join(err, exporter.Shutdown(ctx))
	}
	provider := sdktrace.NewTracerProvider(sdktrace.WithResource(res), sdktrace.WithBatcher(exporter))
	otel.SetTracerProvider(provider)
	return func(ctx context.Context) error {
		flush, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		return provider.Shutdown(flush)
	}, nil
}
