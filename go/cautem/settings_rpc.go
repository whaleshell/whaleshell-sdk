package cautem

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"

	openshell "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1"
)

// GetSetting reads a registered gateway setting through OpenShell RPC.
func (c *Client) GetSetting(ctx context.Context, key string) (string, error) {
	client, err := c.openShellClient()
	if err != nil {
		return "", err
	}
	config, err := client.Config().GetGateway(ctx)
	if err != nil {
		return "", fmt.Errorf("get OpenShell gateway settings: %w", err)
	}
	value, ok := config.Settings[key]
	if !ok || value.Type == "" {
		return "", fmt.Errorf("setting %q not found", key)
	}
	return settingString(value), nil
}

// ListSettings lists configured gateway settings through OpenShell RPC.
func (c *Client) ListSettings(ctx context.Context) (map[string]string, error) {
	client, err := c.openShellClient()
	if err != nil {
		return nil, err
	}
	config, err := client.Config().GetGateway(ctx)
	if err != nil {
		return nil, fmt.Errorf("list OpenShell gateway settings: %w", err)
	}
	result := make(map[string]string, len(config.Settings))
	for key, value := range config.Settings {
		if value.Type != "" {
			result[key] = settingString(value)
		}
	}
	return result, nil
}

// PutSetting updates a gateway-global setting through OpenShell RPC.
func (c *Client) PutSetting(ctx context.Context, key, value string) error {
	client, err := c.openShellClient()
	if err != nil {
		return err
	}
	_, err = client.Config().Update(ctx, c.workspace(), &openshell.ConfigUpdate{
		Global: true, SettingKey: key,
		SettingValue: &openshell.SettingValue{Type: openshell.SettingValueString, StringVal: value},
	})
	if err != nil {
		return fmt.Errorf("update OpenShell gateway setting %q: %w", key, err)
	}
	return nil
}

func settingString(value openshell.SettingValue) string {
	switch value.Type {
	case openshell.SettingValueString:
		return value.StringVal
	case openshell.SettingValueBool:
		return strconv.FormatBool(value.BoolVal)
	case openshell.SettingValueInt:
		return strconv.FormatInt(value.IntVal, 10)
	case openshell.SettingValueBytes:
		return base64.StdEncoding.EncodeToString(value.BytesVal)
	default:
		return ""
	}
}
