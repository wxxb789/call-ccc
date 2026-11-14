// Command fixturegen writes the sanitized native session fixtures the call/ccc tests parse.
//
// The fixtures are generated rather than hand-copied so their shape comes from the pinned revision's
// own fixture builders (go.kenn.io/agentsview/internal/testjsonl) instead of from memory, and so the
// committed bytes can be re-derived and compared:
//
//	go run ./internal/ccc/fixturegen
//
// Every string in them is invented; they carry no session content, credential or host path from a
// real machine. The generator is deterministic: running it again must reproduce the committed files
// byte for byte, which internal/ccc/fixtures_test.go asserts.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.kenn.io/agentsview/internal/testjsonl"
)

// Fixture identity. The Claude id is the file's own stem, as Claude Code names its transcripts; the
// Codex id is the session_meta id and the rollout file name's tail, as Codex names its rollouts.
const (
	claudeSessionID = "9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10"
	claudeProject   = "-home-user-code-example-api"
	claudeVersion   = "1.0.30"

	codexSessionID = "0c8e5f21-3d6a-4b7c-9f10-2ab4cd6ef812"
	codexRollout   = "rollout-2026-05-04T09-15-00-" + codexSessionID
	codexCwd       = "/home/user/code/example-api"

	// A line the writer was interrupted in the middle of, which is what a session being appended to
	// while it is read looks like. The parsers must count it as malformed rather than guess a message
	// from it, and an inspection must report that count.
	truncatedLine = `{"type":"assistant","timestamp":"2026-05-04T09:18:00.000Z","message":{"content":[{"type":"text","text":"Reading the ret`
)

// fixtureFiles maps a path relative to the fixtures root onto its content.
func fixtureFiles() map[string]string {
	return map[string]string{
		filepath.Join("claude", claudeProject, claudeSessionID+".jsonl"):  canonicalJSONL(claudeSession()),
		filepath.Join("codex", "2026", "05", "04", codexRollout+".jsonl"): canonicalJSONL(codexSession()),
	}
}

// canonicalJSONL re-encodes every complete line with one encoding, so the committed bytes are
// reproducible. The inherited builders marshal their maps with encoding/json/v2, whose object key
// order is not stable between runs, and a fixture whose bytes changed on every regeneration could not
// be pinned by a test. A line that is not valid JSON is left exactly as it is: fixturegen writes one
// deliberately truncated line to model a transcript captured mid-append.
func canonicalJSONL(content string) string {
	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		var value any
		if err := json.Unmarshal([]byte(trimmed), &value); err != nil {
			continue
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			panic(err)
		}
		lines[index] = string(encoded)
	}
	return strings.Join(lines, "\n") + "\n"
}

// claudeSession is a Claude Code transcript: two turns, a tool call answered by its result, a
// compaction summary boundary, a billable assistant turn with usage, and a truncated final line.
func claudeSession() string {
	toolCall := []map[string]any{{
		"type": "tool_use",
		"id":   "toolu_01Example0000000000000000",
		"name": "Read",
		"input": map[string]any{
			"file_path": filepath.Join(claudeCwd(), "internal/retry/policy.go"),
		},
	}}
	builder := testjsonl.NewSessionBuilder().
		AddRaw(claudeVersionedUser(
			"Summarize the retry policy in internal/retry/policy.go.", "2026-05-04T09:15:00.000Z")).
		AddClaudeAssistantWithUUID(
			"2026-05-04T09:15:04.000Z",
			"I will read the retry policy first.",
			"aaaaaaaa-0000-4000-8000-000000000001",
			"",
		).
		AddRaw(claudeToolUse(toolCall, "2026-05-04T09:15:06.000Z")).
		AddRaw(testjsonl.ClaudeToolResultUserJSON(
			"toolu_01Example0000000000000000",
			"package retry\n\nconst MaxAttempts = 4\n",
			"2026-05-04T09:15:06.500Z",
		)).
		AddClaudeAssistantWithUUID(
			"2026-05-04T09:15:09.000Z",
			"Four attempts with exponential backoff, capped at 30s between attempts.",
			"aaaaaaaa-0000-4000-8000-000000000002",
			"aaaaaaaa-0000-4000-8000-000000000001",
		).
		AddClaudeMetaUser(
			"2026-05-04T09:16:00.000Z",
			"Summary: the retry policy caps attempts at four and backs off exponentially.",
			false, true,
		).
		AddClaudeUserWithSessionID(
			"2026-05-04T09:16:30.000Z",
			"Now check where the backoff cap is configured.",
			claudeSessionID,
			claudeCwd(),
		).
		AddClaudeAssistantUsage(
			"2026-05-04T09:16:36.000Z",
			"The cap is configured in internal/retry/config.go as maxBackoffSeconds.",
			testjsonl.ClaudeAssistantUsage{
				MessageID:    "msg_01Example000000000000000001",
				RequestID:    "req_01Example000000000000000001",
				Model:        "claude-sonnet-4-5",
				InputTokens:  4210,
				OutputTokens: 96,
			},
		).
		AddRaw(truncatedLine)
	return builder.String()
}

// codexSession is a Codex rollout: session metadata, a turn with a function call and its output, a
// token-count event, a compaction boundary, and a second turn.
func codexSession() string {
	return testjsonl.JoinJSONL(
		testjsonl.CodexSessionMetaJSON(codexSessionID, codexCwd, "codex_cli_rs", "2026-05-04T09:15:00.000Z"),
		testjsonl.CodexTurnContextJSON("gpt-5.2-codex", "2026-05-04T09:15:01.000Z"),
		testjsonl.CodexMsgJSON("user", "Where is the retry cap configured?", "2026-05-04T09:15:02.000Z"),
		testjsonl.CodexFunctionCallWithCallIDJSON(
			"shell",
			"call_01Example000000000000000000",
			map[string]any{"command": "grep -rn maxBackoffSeconds internal/retry"},
			"2026-05-04T09:15:05.000Z",
		),
		testjsonl.CodexFunctionCallOutputJSON(
			"call_01Example000000000000000000",
			"internal/retry/config.go:41: maxBackoffSeconds = 30",
			"2026-05-04T09:15:06.000Z",
		),
		testjsonl.CodexMsgJSON("assistant", "It is set in internal/retry/config.go.", "2026-05-04T09:15:07.000Z"),
		testjsonl.CodexTokenCountJSON("2026-05-04T09:15:08.000Z", 3820, 74, 1024),
		codexCompacted(),
		testjsonl.CodexMsgJSON("user", "And the attempt limit?", "2026-05-04T09:16:10.000Z"),
		testjsonl.CodexMsgJSON("assistant", "Four attempts, in internal/retry/policy.go.", "2026-05-04T09:16:12.000Z"),
	)
}

func codexCompacted() string {
	return marshal(map[string]any{
		"type":      "compacted",
		"timestamp": "2026-05-04T09:16:00.000Z",
		"payload": map[string]any{
			"message": "Compacted: the retry cap question was answered from internal/retry/config.go.",
		},
	})
}

// claudeToolUse is a Claude assistant line whose content is a tool_use block. The inherited builder
// covers text and usage turns; a tool_use turn is written here with the same native shape.
func claudeToolUse(content any, timestamp string) string {
	return marshal(map[string]any{
		"type":      "assistant",
		"timestamp": timestamp,
		"version":   claudeVersion,
		"message": map[string]any{
			"content": content,
			"model":   "claude-sonnet-4-5",
		},
	})
}

// claudeVersionedUser is a Claude user line carrying the source version, which older and newer
// Claude Code releases both put on their transcript lines.
func claudeVersionedUser(text, timestamp string) string {
	return marshal(map[string]any{
		"type":      "user",
		"timestamp": timestamp,
		"version":   claudeVersion,
		"sessionId": claudeSessionID,
		"cwd":       claudeCwd(),
		"message":   map[string]any{"content": text},
	})
}

func claudeCwd() string { return "/home/user/code/example-api" }

// marshal writes one JSONL line deterministically (Go maps marshal with sorted keys).
func marshal(value map[string]any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

func main() {
	root := flag.String("root", filepath.Join("internal", "ccc", "testdata", "fixtures"),
		"directory to write the fixtures into")
	flag.Parse()

	files := fixtureFiles()
	relatives := make([]string, 0, len(files))
	for relative := range files {
		relatives = append(relatives, relative)
	}
	sort.Strings(relatives)
	paths := make([]string, 0, len(files))
	for _, relative := range relatives {
		content := files[relative]
		target := filepath.Join(*root, relative)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			fmt.Fprintln(os.Stderr, "fixturegen:", err)
			os.Exit(1)
		}
		if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "fixturegen:", err)
			os.Exit(1)
		}
		paths = append(paths, target)
	}
	for _, path := range paths {
		fmt.Println("wrote", path)
	}
}
