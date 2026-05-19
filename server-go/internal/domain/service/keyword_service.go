package service

import "strings"

// promptKeywords maps each prompt-intent category to its trigger keywords.
// The keyword set (including the Chinese entries) is hard-coded and MUST stay
// byte-identical to the Rust client and Java server implementations.
var promptKeywords = map[string][]string{
	"generate":  {"generate", "create", "write", "implement", "add", "build", "make", "生成", "创建", "新增", "写", "实现", "构建"},
	"fix_debug": {"fix", "bug", "error", "debug", "issue", "wrong", "broken", "修复", "错误", "调试", "修", "问题", "故障", "报错"},
	"refactor":  {"refactor", "clean", "improve", "optimize", "restructure", "重构", "优化", "改进", "简化", "整理"},
	"explain":   {"explain", "what", "how", "why", "describe", "解释", "什么", "怎么", "为什么", "说明"},
	"test":      {"test", "spec", "mock", "coverage", "unit", "测试", "单元测试", "集成测试", "断言", "覆盖"},
}

// classifyPrompt returns the category for a prompt summary.
// Priority: fix_debug > test > refactor > explain > generate > other
func classifyPrompt(s string) string {
	lower := strings.ToLower(s)
	if matchesAny(lower, promptKeywords["fix_debug"]...) {
		return "fix_debug"
	}
	if matchesAny(lower, promptKeywords["test"]...) {
		return "test"
	}
	if matchesAny(lower, promptKeywords["refactor"]...) {
		return "refactor"
	}
	if matchesAny(lower, promptKeywords["explain"]...) {
		return "explain"
	}
	if matchesAny(lower, promptKeywords["generate"]...) {
		return "generate"
	}
	return "other"
}

// ComputePromptPatterns classifies prompt_summary text into intent categories.
func ComputePromptPatterns(records []RawRecord) map[string]int64 {
	patterns := map[string]int64{
		"generate":  0,
		"fix_debug": 0,
		"refactor":  0,
		"explain":   0,
		"test":      0,
		"other":     0,
	}
	for _, r := range records {
		if r.PromptSummary == nil || *r.PromptSummary == "" {
			continue
		}
		patterns[classifyPrompt(*r.PromptSummary)]++
	}
	return patterns
}

func matchesAny(s string, keywords ...string) bool {
	for _, kw := range keywords {
		if strings.Contains(s, kw) {
			return true
		}
	}
	return false
}
