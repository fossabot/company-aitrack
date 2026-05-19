package service_test

import (
	"testing"

	"github.com/aitrack/server/internal/domain/service"
)

// ─── DetectLanguage unit tests ─────────────────────────────────────────────

func TestDetectLanguage(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		// empty path
		{"", "Other"},

		// known extensions
		{"src/app.py", "Python"},
		{"src/app.ts", "TypeScript"},
		{"src/app.tsx", "TypeScript"},
		{"src/app.js", "JavaScript"},
		{"src/app.jsx", "JavaScript"},
		{"src/Main.java", "Java"},
		{"internal/handler/profile.go", "Go"},
		{"src/lib.rs", "Rust"},
		{"src/main.cpp", "C/C++"},
		{"src/util.cc", "C/C++"},
		{"src/util.c", "C/C++"},
		{"src/App.cs", "C#"},
		{"lib/helper.rb", "Ruby"},
		{"src/index.php", "PHP"},
		{"App.swift", "Swift"},
		{"app/Main.kt", "Kotlin"},
		{"build.kts", "Kotlin"},
		{"src/Main.scala", "Scala"},
		{"components/App.vue", "Vue"},
		{"index.html", "HTML"},
		{"index.htm", "HTML"},
		{"styles/main.css", "CSS"},
		{"styles/main.scss", "CSS"},
		{"styles/main.sass", "CSS"},
		{"styles/main.less", "CSS"},
		{"schema.sql", "SQL"},
		{"deploy.sh", "Shell"},
		{"deploy.bash", "Shell"},
		{"deploy.zsh", "Shell"},
		{"config.yaml", "YAML"},
		{"config.yml", "YAML"},
		{"package.json", "JSON"},
		{"pom.xml", "XML"},
		{"README.md", "Docs"},
		{"API.rst", "Docs"},
		{"CHANGELOG.txt", "Docs"},

		// unknown extension
		{"src/file.xyz", "Other"},
		{"Makefile", "Other"},
	}

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			got := service.DetectLanguage(tc.path)
			if got != tc.want {
				t.Errorf("DetectLanguage(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

// ─── ComputePromptPatterns unit tests ─────────────────────────────────────

func TestComputePromptPatterns(t *testing.T) {
	t.Run("generate keyword matches", func(t *testing.T) {
		ps := "implement new auth feature"
		records := []service.RawRecord{{PromptSummary: &ps}}
		patterns := service.ComputePromptPatterns(records)
		if patterns["generate"] != 1 {
			t.Errorf("generate = %d, want 1", patterns["generate"])
		}
		if patterns["fix_debug"] != 0 {
			t.Errorf("fix_debug = %d, want 0", patterns["fix_debug"])
		}
	})

	t.Run("fix_debug keyword matches", func(t *testing.T) {
		ps := "fix the login bug"
		records := []service.RawRecord{{PromptSummary: &ps}}
		patterns := service.ComputePromptPatterns(records)
		if patterns["fix_debug"] != 1 {
			t.Errorf("fix_debug = %d, want 1", patterns["fix_debug"])
		}
		if patterns["generate"] != 0 {
			t.Errorf("generate = %d, want 0", patterns["generate"])
		}
	})

	t.Run("nil prompt_summary is skipped", func(t *testing.T) {
		records := []service.RawRecord{{PromptSummary: nil}}
		patterns := service.ComputePromptPatterns(records)
		for k, v := range patterns {
			if v != 0 {
				t.Errorf("patterns[%q] = %d, want 0", k, v)
			}
		}
	})

	t.Run("other category when no keyword matches", func(t *testing.T) {
		ps := "do something vague"
		records := []service.RawRecord{{PromptSummary: &ps}}
		patterns := service.ComputePromptPatterns(records)
		if patterns["other"] != 1 {
			t.Errorf("other = %d, want 1", patterns["other"])
		}
	})

	t.Run("Chinese generate keyword matches", func(t *testing.T) {
		ps := "帮我生成一个 REST API"
		records := []service.RawRecord{{PromptSummary: &ps}}
		patterns := service.ComputePromptPatterns(records)
		if patterns["generate"] != 1 {
			t.Errorf("generate = %d, want 1", patterns["generate"])
		}
		if patterns["other"] != 0 {
			t.Errorf("other = %d, want 0", patterns["other"])
		}
	})

	t.Run("Chinese fix_debug keyword matches", func(t *testing.T) {
		ps := "修复这个错误"
		records := []service.RawRecord{{PromptSummary: &ps}}
		patterns := service.ComputePromptPatterns(records)
		if patterns["fix_debug"] != 1 {
			t.Errorf("fix_debug = %d, want 1", patterns["fix_debug"])
		}
		if patterns["other"] != 0 {
			t.Errorf("other = %d, want 0", patterns["other"])
		}
	})

	t.Run("Chinese test keyword matches", func(t *testing.T) {
		ps := "写单元测试"
		records := []service.RawRecord{{PromptSummary: &ps}}
		patterns := service.ComputePromptPatterns(records)
		if patterns["test"] != 1 {
			t.Errorf("test = %d, want 1", patterns["test"])
		}
		if patterns["other"] != 0 {
			t.Errorf("other = %d, want 0", patterns["other"])
		}
	})
}

// ─── ComputeCommentDensity unit tests ──────────────────────────────────────

func TestComputeCommentDensity(t *testing.T) {
	t.Run("empty records returns 0.0", func(t *testing.T) {
		got := service.ComputeCommentDensity(nil)
		if got != 0.0 {
			t.Errorf("got %v, want 0.0", got)
		}
	})

	t.Run("empty diffHunk is skipped", func(t *testing.T) {
		got := service.ComputeCommentDensity([]service.RawRecord{
			{DiffHunk: ""},
			{DiffHunk: ""},
		})
		if got != 0.0 {
			t.Errorf("got %v, want 0.0", got)
		}
	})

	t.Run("one comment line one code line gives 0.5", func(t *testing.T) {
		diff := "+// this is a comment\n+someCode()"
		got := service.ComputeCommentDensity([]service.RawRecord{
			{DiffHunk: diff},
		})
		const want = 0.5
		if got != want {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("only code lines gives 0.0", func(t *testing.T) {
		diff := "+foo()\n+bar()\n+baz()"
		got := service.ComputeCommentDensity([]service.RawRecord{
			{DiffHunk: diff},
		})
		if got != 0.0 {
			t.Errorf("got %v, want 0.0", got)
		}
	})

	t.Run("only comment lines gives 1.0", func(t *testing.T) {
		diff := "+// line1\n+# line2\n+/* line3 */"
		got := service.ComputeCommentDensity([]service.RawRecord{
			{DiffHunk: diff},
		})
		if got != 1.0 {
			t.Errorf("got %v, want 1.0", got)
		}
	})

	t.Run("removal lines and +++ header are ignored", func(t *testing.T) {
		diff := "+++ b/file.go\n-removed line\n+// comment\n+code"
		got := service.ComputeCommentDensity([]service.RawRecord{
			{DiffHunk: diff},
		})
		const want = 0.5
		if got != want {
			t.Errorf("got %v, want %v", got, want)
		}
	})
}

// ─── Percentile unit tests ─────────────────────────────────────────────────

func TestPercentile(t *testing.T) {
	if got := service.Percentile(nil, 0.5); got != 0 {
		t.Errorf("Percentile(nil) = %d, want 0", got)
	}
	sorted := []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	if got := service.Percentile(sorted, 0.9); got != 10 {
		t.Errorf("Percentile(p90) = %d, want 10", got)
	}
	if got := service.Percentile(sorted, 1.0); got != 10 {
		t.Errorf("Percentile(p100 clamps) = %d, want 10", got)
	}
}
