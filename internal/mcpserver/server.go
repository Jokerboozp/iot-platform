package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"iot-platform/internal/auth"
	"iot-platform/internal/core"
	"iot-platform/internal/model"
	"iot-platform/internal/ports"
)

func New(engine *core.Engine) http.Handler {
	s := server.NewMCPServer("iot-platform-tools", "1.0.0", server.WithToolCapabilities(false), server.WithInstructions("只读查询消防物联网数据；不能执行 SQL、控制设备或自动启用规则。"), server.WithRecovery())
	s.AddTool(mcp.NewTool("query_device_latest", mcp.WithDescription("查询当前租户内某设备的最新在线和业务状态"), mcp.WithString("deviceId", mcp.Required(), mcp.Description("设备 ID"))), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		tenant, err := tenantFrom(ctx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		v, err := engine.Repo.GetDeviceState(ctx, tenant, req.GetString("deviceId", ""))
		return auditedResult(ctx, engine, "query_device_latest", map[string]any{"deviceId": req.GetString("deviceId", "")}, v, err)
	})
	s.AddTool(mcp.NewTool("query_alarm_list", mcp.WithDescription("按状态、等级、设备和时间范围查询当前租户告警"), mcp.WithString("deviceId"), mcp.WithString("status"), mcp.WithString("level"), mcp.WithNumber("start"), mcp.WithNumber("end"), mcp.WithNumber("limit")), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		tenant, err := tenantFrom(ctx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		v, err := engine.Repo.ListAlarms(ctx, ports.AlarmFilter{TenantID: tenant, DeviceID: req.GetString("deviceId", ""), Status: req.GetString("status", ""), Level: req.GetString("level", ""), Start: int64(req.GetInt("start", 0)), End: int64(req.GetInt("end", 0)), Limit: req.GetInt("limit", 100)})
		return auditedResult(ctx, engine, "query_alarm_list", map[string]any{"deviceId": req.GetString("deviceId", ""), "status": req.GetString("status", ""), "level": req.GetString("level", "")}, v, err)
	})
	s.AddTool(mcp.NewTool("query_property_history", mcp.WithDescription("查询当前租户内设备属性历史趋势"), mcp.WithString("deviceId", mcp.Required()), mcp.WithString("propertyCode", mcp.Required()), mcp.WithNumber("start", mcp.Required()), mcp.WithNumber("end", mcp.Required()), mcp.WithNumber("limit")), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		tenant, err := tenantFrom(ctx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		v, err := engine.Repo.PropertyHistory(ctx, tenant, req.GetString("deviceId", ""), req.GetString("propertyCode", ""), int64(req.GetInt("start", 0)), int64(req.GetInt("end", int(time.Now().UnixMilli()))), req.GetInt("limit", 1000))
		return auditedResult(ctx, engine, "query_property_history", map[string]any{"deviceId": req.GetString("deviceId", ""), "propertyCode": req.GetString("propertyCode", "")}, v, err)
	})
	s.AddTool(mcp.NewTool("query_similar_alarms", mcp.WithDescription("查询同设备、同类型的历史告警"), mcp.WithString("deviceId", mcp.Required()), mcp.WithNumber("limit")), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		tenant, err := tenantFrom(ctx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		v, err := engine.Repo.ListAlarms(ctx, ports.AlarmFilter{TenantID: tenant, DeviceID: req.GetString("deviceId", ""), Limit: req.GetInt("limit", 20)})
		return auditedResult(ctx, engine, "query_similar_alarms", map[string]any{"deviceId": req.GetString("deviceId", "")}, v, err)
	})
	s.AddTool(mcp.NewTool("query_knowledge_base", mcp.WithDescription("检索当前租户的设备手册、SOP 和维修知识"), mcp.WithString("question", mcp.Required()), mcp.WithNumber("limit")), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		tenant, err := tenantFrom(ctx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if engine.KB == nil {
			return mcp.NewToolResultError("knowledge base disabled"), nil
		}
		v, err := engine.KB.Search(ctx, tenant, req.GetString("question", ""), req.GetInt("limit", 5))
		return auditedResult(ctx, engine, "query_knowledge_base", map[string]any{"question": req.GetString("question", "")}, v, err)
	})
	s.AddTool(mcp.NewTool("create_rule_draft", mcp.WithDescription("把自然语言告警要求转换为禁用状态的规则草稿；必须人工审核后才能启用"), mcp.WithString("inputText", mcp.Required())), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		tenant, err := tenantFrom(ctx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		v, err := engine.AI.RuleDraft(ctx, tenant, req.GetString("inputText", ""))
		return auditedResult(ctx, engine, "create_rule_draft", map[string]any{"inputText": req.GetString("inputText", "")}, v, err)
	})
	return server.NewStreamableHTTPServer(s, server.WithStateLess(true), server.WithEndpointPath("/mcp"))
}

func tenantFrom(ctx context.Context) (string, error) {
	c, ok := auth.ClaimsFromContext(ctx)
	if !ok || c.TenantID == "" {
		return "", fmt.Errorf("authenticated tenant context is required")
	}
	return c.TenantID, nil
}
func result(v any, err error) (*mcp.CallToolResult, error) {
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	b, _ := json.Marshal(v)
	return mcp.NewToolResultText(string(b)), nil
}
func auditedResult(ctx context.Context, engine *core.Engine, tool string, input map[string]any, v any, err error) (*mcp.CallToolResult, error) {
	c, _ := auth.ClaimsFromContext(ctx)
	entry := model.AIToolCallLog{ID: fmt.Sprintf("tool_%d", time.Now().UnixNano()), TenantID: c.TenantID, Actor: c.Username, Tool: tool, Input: input, Output: v, Success: err == nil, CreatedAt: time.Now().UnixMilli()}
	if err != nil {
		entry.Error = err.Error()
	}
	_ = engine.Repo.SaveAIToolCall(ctx, entry)
	return result(v, err)
}
