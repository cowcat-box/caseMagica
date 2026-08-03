package agent

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/schema"

	"casemagica/config"
)

// GenerateStandaloneSpec 描述一次有界单轮模型调用（无工具）。
// 用于与 Agent 主运行管道无关的独立生成场景（如续写推演候选、自动化评估）。
type GenerateStandaloneSpec struct {
	AgentKind   string // 用于模型选择与 trace 归类的 Agent 类型
	Source      string // trace 来源标识
	Mode        string // trace 模式标识
	System      string // 内建 system prompt 内容（会经过 protectedSystemInstruction 包装）
	Instruction string // 用户指令/上下文
	JSONMode    bool   // 是否要求 JSON 输出
}

// GenerateStandalone 执行一次有界单轮模型调用并返回输出文本。
// 内部封装 trace 初始化、JSON 模式与纯文本重试（对齐 tool_agent 的 json_mode 容错路径）。
func GenerateStandalone(ctx context.Context, cfg *config.Config, spec GenerateStandaloneSpec) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("配置不存在")
	}
	instruction := strings.TrimSpace(spec.Instruction)
	if instruction == "" {
		return "", fmt.Errorf("指令为空")
	}
	var runErr error
	traceCtx, finishTrace := withStandaloneRunTrace(ctx, cfg, spec.AgentKind, spec.Source, spec.Mode, map[string]any{
		"instruction_chars": len([]rune(instruction)),
		"json_mode":         spec.JSONMode,
	})
	defer func() { finishTrace(runErr) }()

	jsonModelCfg := chatModelConfigForAgent(cfg, spec.AgentKind)
	if spec.JSONMode {
		jsonModelCfg.ResponseFormat = &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONObject,
		}
	}
	systemPrompt := protectedSystemInstruction(cfg, spec.AgentKind, spec.System)
	output, err := generateStandaloneOnce(traceCtx, cfg, spec, jsonModelCfg, systemPrompt, instruction, "json_mode")
	if err == nil {
		return output, nil
	}
	if traceCtx.Err() != nil {
		runErr = err
		return "", err
	}
	if spec.JSONMode {
		log.Printf("[standalone-generate] json_mode failed, retry without response_format source=%s mode=%s err=%v", spec.Source, spec.Mode, err)
		plainModelCfg := chatModelConfigForAgent(cfg, spec.AgentKind)
		output, retryErr := generateStandaloneOnce(traceCtx, cfg, spec, plainModelCfg, systemPrompt, instruction, "plain_text_retry")
		if retryErr != nil {
			runErr = retryErr
			return "", retryErr
		}
		return output, nil
	}
	runErr = err
	return "", err
}

func generateStandaloneOnce(ctx context.Context, cfg *config.Config, spec GenerateStandaloneSpec, modelCfg openai.ChatModelConfig, systemPrompt, instruction, attempt string) (string, error) {
	cm, err := openai.NewChatModel(ctx, &modelCfg)
	if err != nil {
		log.Printf("[standalone-generate] create model failed source=%s mode=%s attempt=%s err=%v", spec.Source, spec.Mode, attempt, err)
		return "", fmt.Errorf("创建生成模型失败: %w", err)
	}
	messages := []*schema.Message{
		schema.SystemMessage(systemPrompt),
		schema.UserMessage(instruction),
	}
	span, callID, traceCtx := beginLLMCallTrace(ctx, spec.AgentKind, spec.Source, spec.Mode+"_"+attempt, modelCfg, messages, nil, false)
	msg, err := cm.Generate(traceCtx, messages)
	if err != nil {
		finishLLMCallTrace(span, callID, spec.AgentKind, spec.Source, spec.Mode+"_"+attempt, modelCfg.Model, 0, nil, err, nil)
		return "", fmt.Errorf("生成失败: %w", err)
	}
	if msg == nil || strings.TrimSpace(msg.Content) == "" {
		finishLLMCallTrace(span, callID, spec.AgentKind, spec.Source, spec.Mode+"_"+attempt, modelCfg.Model, 0, nil, fmt.Errorf("模型返回为空"), nil)
		return "", fmt.Errorf("模型返回为空")
	}
	finishLLMCallTrace(span, callID, spec.AgentKind, spec.Source, spec.Mode+"_"+attempt, modelCfg.Model, 0, msg, nil, nil)
	log.Printf("[standalone-generate] done source=%s mode=%s attempt=%s output_chars=%d", spec.Source, spec.Mode, attempt, len([]rune(msg.Content)))
	return msg.Content, nil
}
