package cautem

import (
	"context"
	"fmt"

	controlv1 "github.com/cautem/cautem-gateway/api/gen/cautem/control/v1"
	gc "github.com/cautem/cautem-sdk/internal/gatewayclient"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GetInference reads the gateway inference route through the Control RPC.
func (c *Client) GetInference(ctx context.Context) (gc.InferenceRoute, error) {
	conn, err := c.controlConn()
	if err != nil {
		return gc.InferenceRoute{}, err
	}
	response, err := controlv1.NewInferenceServiceClient(conn).GetInferenceRoute(ctx, &controlv1.GetInferenceRouteRequest{})
	if err != nil {
		return gc.InferenceRoute{}, fmt.Errorf("get control inference route: %w", err)
	}
	return gc.InferenceRoute{Provider: response.GetProvider(), Model: response.GetModel(), TimeoutSec: int(response.GetTimeoutSec()), Version: int(response.GetResourceVersion())}, nil
}

// PutInference updates the gateway inference route with optimistic concurrency.
func (c *Client) PutInference(ctx context.Context, route gc.InferenceRoute) (gc.InferenceRoute, error) {
	if route.Version < 0 {
		return gc.InferenceRoute{}, fmt.Errorf("inference route version must not be negative")
	}
	conn, err := c.controlConn()
	if err != nil {
		return gc.InferenceRoute{}, err
	}
	client := controlv1.NewInferenceServiceClient(conn)
	expected := uint64(route.Version)
	if route.Version == 0 {
		current, readErr := client.GetInferenceRoute(ctx, &controlv1.GetInferenceRouteRequest{})
		if readErr == nil {
			expected = current.GetResourceVersion()
		} else if status.Code(readErr) != codes.NotFound {
			return gc.InferenceRoute{}, fmt.Errorf("read control inference route before update: %w", readErr)
		}
	}
	response, err := client.UpdateInferenceRoute(ctx, &controlv1.UpdateInferenceRouteRequest{
		Provider: route.Provider, Model: route.Model, TimeoutSec: int32(route.TimeoutSec), ExpectedResourceVersion: expected,
	})
	if err != nil {
		return gc.InferenceRoute{}, fmt.Errorf("update control inference route: %w", err)
	}
	route.Version = int(response.GetResourceVersion())
	return route, nil
}

// DeleteInference clears the gateway inference route using its current version.
func (c *Client) DeleteInference(ctx context.Context) error {
	conn, err := c.controlConn()
	if err != nil {
		return err
	}
	client := controlv1.NewInferenceServiceClient(conn)
	current, err := client.GetInferenceRoute(ctx, &controlv1.GetInferenceRouteRequest{})
	expected := uint64(0)
	if err == nil {
		expected = current.GetResourceVersion()
	} else if status.Code(err) != codes.NotFound {
		return fmt.Errorf("read control inference route before delete: %w", err)
	}
	if _, err := client.ClearInferenceRoute(ctx, &controlv1.ClearInferenceRouteRequest{ExpectedResourceVersion: expected}); err != nil {
		return fmt.Errorf("clear control inference route: %w", err)
	}
	return nil
}
