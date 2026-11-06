package assets

import (
	"context"
	"strconv"
	"strings"

	"autoadmin/internal/shared/logmacro"
)

type InstancePort struct {
	Deployment      int64  `json:"deployment"`
	InstanceName    string `json:"instance_name"`
	ResolvedPort    string `json:"resolved_port"`
	ResolutionError string `json:"resolution_error,omitempty"`
}

func resolveConfiguredPort(expression string, values map[string]string) (string, string) {
	resolved, err := logmacro.ResolveChecked(expression, values)
	if err != nil {
		return resolved, err.Error()
	}
	resolved = strings.TrimSpace(resolved)
	number, err := strconv.Atoi(resolved)
	if err != nil || number < 1 || number > 65535 {
		return resolved, "解析结果必须为 1–65535 的整数"
	}
	return strconv.Itoa(number), ""
}

func (r *Repository) resolveServicePorts(ctx context.Context, service ApplicationService, ports []TemplatePort) error {
	hasMacro := false
	for index := range ports {
		ports[index].ResolvedPort, ports[index].ResolutionError = resolveConfiguredPort(string(ports[index].Port), nil)
		hasMacro = hasMacro || strings.Contains(string(ports[index].Port), "${")
	}
	if !hasMacro {
		return nil
	}
	rows, err := r.queries.ListServicePortMacroContexts(ctx, service.ID)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return ErrInvalidRelation
	}
	serviceValues := logmacro.Merge(
		logmacro.TemplateDefaults(string(rows[0].MacroDefinitions)),
		logmacro.ParseValues(string(service.MacroValues)),
	)
	if _, exists := serviceValues["APP_HOME"]; !exists && rows[0].AppHome != "" {
		serviceValues["APP_HOME"] = rows[0].AppHome
	}
	for index := range ports {
		port := &ports[index]
		port.ResolvedPort, port.ResolutionError = resolveConfiguredPort(string(port.Port), serviceValues)
		var instances []InstancePort
		different := false
		for _, row := range rows {
			if !row.DeploymentID.Valid {
				continue
			}
			values := logmacro.Merge(serviceValues, logmacro.InstanceValues(string(row.RuntimeVariables), row.AppHome))
			resolved, problem := resolveConfiguredPort(string(port.Port), values)
			instances = append(instances, InstancePort{
				Deployment: row.DeploymentID.Int64, InstanceName: row.InstanceName.String,
				ResolvedPort: resolved, ResolutionError: problem,
			})
			different = different || resolved != port.ResolvedPort || problem != port.ResolutionError
		}
		// Show every instance when any override changes the outcome; omitting inheriting
		// instances would make the remaining service value ambiguous.
		if different {
			port.InstancePorts = instances
		}
	}
	return nil
}
