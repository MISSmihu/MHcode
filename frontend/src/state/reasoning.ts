import type { ReasoningLevel, ReasoningOption } from "../types";

export const defaultReasoningLevel: ReasoningLevel = "ultra";

export const reasoningLabels: Record<ReasoningLevel, string> = {
  low: "低",
  medium: "中",
  high: "高",
  ultra: "超高",
};

export const reasoningOptions: ReasoningOption[] = [
  {
    id: "low",
    label: reasoningLabels.low,
    description: "简单问答、轻量编辑、低成本优先",
    budget: {
      maxToolCalls: 3,
      contextPolicy: "minimal",
      cachePolicy: "reuse-prefix",
      planner: false,
    },
  },
  {
    id: "medium",
    label: reasoningLabels.medium,
    description: "普通代码修改、单文件任务",
    budget: {
      maxToolCalls: 8,
      contextPolicy: "task-summary",
      cachePolicy: "reuse-prefix",
      planner: false,
    },
  },
  {
    id: "high",
    label: reasoningLabels.high,
    description: "跨文件修改、复杂 bug、测试修复",
    budget: {
      maxToolCalls: 16,
      contextPolicy: "expanded",
      cachePolicy: "stable-prefix",
      planner: true,
    },
  },
  {
    id: "ultra",
    label: reasoningLabels.ultra,
    description: "协议设计、Agent 架构、发布级检查",
    budget: {
      maxToolCalls: 32,
      contextPolicy: "full-relevant",
      cachePolicy: "strict-stable-prefix",
      planner: true,
    },
  },
];
