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
	"path"
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

	// The second Claude fixture carries the shapes a conversion has to report: a persisted reasoning
	// trace, a subagent's turn recorded in the same transcript, a provider system record, a tool result
	// the source marked as an error, and a tool result the provider wrote on the same record as a system
	// reminder. It carries no compaction boundary, so the ordinary turns a resumed target reads from it
	// are the whole conversation.
	claudeToolingSessionID = "b6d93f07-2f5c-4a11-8e0d-3c7a91d4b2ef"
	claudeToolingProject   = "-home-user-code-tooling"
	claudeToolingCwd       = "/home/user/code/tooling"

	codexSessionID = "0c8e5f21-3d6a-4b7c-9f10-2ab4cd6ef812"
	codexRollout   = "rollout-2026-05-04T09-15-00-" + codexSessionID
	codexCwd       = "/home/user/code/example-api"

	// A third Codex fixture whose one tool call is a patch touching two files. The inherited reader
	// projects one `apply_patch` call into one call per file it names (internal/parser's
	// handleFunctionCall, so file-keyed views can group by path), all sharing the call's own id and its
	// input. It is the source shape a writer must coalesce rather than write once per projection: a
	// target whose format carries one block per call id reads fewer calls back than such a file claims.
	codexPatchSessionID = "5b2d7c14-6e8a-4f31-b0c5-9d7e21a4c8fe"
	codexPatchRollout   = "rollout-2026-05-06T10-30-00-" + codexPatchSessionID
	codexPatchCwd       = "/home/user/code/patchwork"
	codexPatchCallID    = "call_02Example000000000000000000"
	codexPatchBody      = "*** Begin Patch\n" +
		"*** Update File: internal/retry/config.go\n" +
		"*** Update File: internal/retry/policy.go\n" +
		"*** End Patch"

	// A line the writer was interrupted in the middle of, which is what a session being appended to
	// while it is read looks like. The parsers must count it as malformed rather than guess a message
	// from it, and an inspection must report that count.
	truncatedLine = `{"type":"assistant","timestamp":"2026-05-04T09:18:00.000Z","message":{"content":[{"type":"text","text":"Reading the ret`
)

// Growth-fragment identity: one further turn a harness wrote into a session that is already a fixture, in
// each provider's own record shape, and a record a harness had not finished writing when the file was
// copied.
//
// A growth fragment is *appended* to a copy of a committed fixture, which is how a test or the native check
// makes one source session gain history without the committed bytes moving: the goal this serves is about
// converting the same growing source twice. The complete turn exists once per accepted source provider
// because the no-line-feed rule is pinned end to end in both directions: the fragment's own record shape is
// what that provider's reader has to read whole when the file ends without a line feed. The fragment's
// strings are invented like every other fixture string, and its one fact (the retry budget) appears in no
// other fixture, so a target that continued from the grown snapshot can only have read it there.
const (
	claudeGrowthUserUUID      = "bbbbbbbb-0000-4000-8000-000000000011"
	claudeGrowthAssistantUUID = "bbbbbbbb-0000-4000-8000-000000000012"

	// The appended turn's own fact, quoted by the evidence record's native run when a real turn answers
	// from the grown snapshot. Both providers' fragments carry it: the same question and the same answer,
	// recorded in each provider's own shape.
	growthQuestion = "What budget do retries share per session?"
	growthAnswer   = "The retry budget is 120 seconds per session, held in internal/retry/budget.go."

	codexGrowthTail = `{"payload":{"content":[{"text":"Which file holds the retry budget?","type":"input_text"}],"role":"user"},"timestamp":"2026-05-04T09:16:20.000Z","type":"response_item`

	claudeGrowthTail = `{"type":"assistant","timestamp":"2026-05-04T09:20:04.000Z","message":{"content":[{"type":"text","text":"The retry budget is 120 sec`
)

// growthFiles maps a path relative to the growth root onto its content: the fragments a test or the native
// check appends to a *copy* of a committed fixture, never to the fixture itself.
//
// The two tails are written as literals rather than through the inherited builders, and deliberately: a
// fragment is a byte prefix of a record, so it cannot be re-encoded by canonicalJSONL (which is what makes
// the complete fixtures reproducible), and the builders' own marshalling is not byte-stable between runs.
// A prefix of a shape is the one fixture a literal is the honest form of — the same reason the fixture
// above carries its truncated last line as a literal.
func growthFiles() map[string]string {
	return map[string]string{
		"claude-complete-turn.jsonl":     canonicalJSONL(claudeGrowthTurn()),
		"codex-complete-turn.jsonl":      canonicalJSONL(codexGrowthTurn()),
		"claude-unfinished-record.jsonl": claudeGrowthTail,
		"codex-unfinished-record.jsonl":  codexGrowthTail,
	}
}

// claudeGrowthTurn is one further Claude Code turn as the harness records it: a user record carrying the
// session's own identity and working directory, and the assistant record that answered it, chained to each
// other by uuid and parentUuid.
func claudeGrowthTurn() string {
	builder := testjsonl.NewSessionBuilder().
		AddClaudeUserWithUUID(
			"2026-05-04T09:20:00.000Z",
			growthQuestion,
			claudeGrowthUserUUID,
			"",
			claudeCwd(),
		).
		AddClaudeAssistantWithUUID(
			"2026-05-04T09:20:04.000Z",
			growthAnswer,
			claudeGrowthAssistantUUID,
			claudeGrowthUserUUID,
		)
	return builder.String()
}

// codexGrowthTurn is one further Codex turn as a rollout records it: the user's response_item and the
// assistant's answer to it, in the shape the committed rollout fixture carries -- the inherited builder's
// own `response_item` records, one per message. A case removes this fragment's final line feed, so its last
// record arrives complete and unterminated, which is the shape the Codex reader has to read whole.
func codexGrowthTurn() string {
	return testjsonl.JoinJSONL(
		testjsonl.CodexMsgJSON("user", growthQuestion, "2026-05-04T09:16:20.000Z"),
		testjsonl.CodexMsgJSON("assistant", growthAnswer, "2026-05-04T09:16:24.000Z"),
	)
}

// fixtureFiles maps a path relative to the fixtures root onto its content.
func fixtureFiles() map[string]string {
	return map[string]string{
		filepath.Join("claude", claudeProject, claudeSessionID+".jsonl"):               canonicalJSONL(claudeSession()),
		filepath.Join("claude", claudeToolingProject, claudeToolingSessionID+".jsonl"): canonicalJSONL(claudeToolingSession()),
		filepath.Join("codex", "2026", "05", "04", codexRollout+".jsonl"):              canonicalJSONL(codexSession()),
		filepath.Join("codex", "2026", "05", "06", codexPatchRollout+".jsonl"):         canonicalJSONL(codexPatchSession()),
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
			// path.Join, not filepath.Join: this string is the wire form a transcript holds, which is
			// a POSIX path on every platform, and the committed bytes have to be regenerable
			// byte-for-byte wherever the generator runs.
			"file_path": path.Join(claudeCwd(), "internal/retry/policy.go"),
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
		AddRaw(claudeToolUse(toolCall, "2026-05-04T09:15:06.000Z", "")).
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

// claudeToolingSession is a Claude Code transcript for the tool-heavy shapes a conversion reports: a
// persisted reasoning trace beside its dialogue, one recorded alone, two tool calls whose results arrive
// as their own user lines (one of them recorded as an error), a third whose recorded result is itself a
// JSON string literal and arrives on a record the provider wrote as a system reminder, a subagent turn
// marked isSidechain, and a provider system record.
func claudeToolingSession() string {
	readCall := []map[string]any{{
		"type": "tool_use",
		"id":   "toolu_01Tooling00000000000000001",
		"name": "Read",
		"input": map[string]any{
			"file_path": path.Join(claudeToolingCwd, "internal/retry/policy.go"),
		},
	}}
	bashCall := []map[string]any{{
		"type": "tool_use",
		"id":   "toolu_01Tooling00000000000000002",
		"name": "Bash",
		"input": map[string]any{
			"command":     "grep -rn maxBackoffSeconds internal/retry",
			"description": "find where the backoff cap is configured",
		},
	}}
	jsonCall := []map[string]any{{
		"type": "tool_use",
		"id":   "toolu_01Tooling00000000000000003",
		"name": "Bash",
		"input": map[string]any{
			"command":     "python3 -c 'import json; print(json.dumps(\"maxBackoffSeconds\"))'",
			"description": "confirm the constant's name as JSON",
		},
	}}
	builder := testjsonl.NewSessionBuilder().
		AddRaw(claudeToolingUser(
			"Check the retry policy and tell me the attempt limit.", "2026-05-05T10:00:00.000Z")).
		AddRaw(claudeThinkingAssistant(
			"2026-05-05T10:00:03.000Z",
			"Reading the policy before answering.",
			"I will read the policy file first.",
			"bbbbbbbb-0000-4000-8000-000000000001",
		)).
		AddRaw(claudeToolUse(readCall, "2026-05-05T10:00:05.000Z", "bbbbbbbb-0000-4000-8000-000000000002")).
		AddRaw(testjsonl.ClaudeToolResultUserJSON(
			"toolu_01Tooling00000000000000001",
			"package retry\n\nconst MaxAttempts = 4\n",
			"2026-05-05T10:00:05.500Z",
		)).
		AddRaw(testjsonl.ClaudeAssistantJSON(
			[]map[string]any{{"type": "text", "text": "The attempt limit is four."}},
			"2026-05-05T10:00:07.000Z",
		)).
		AddRaw(claudeToolUse(bashCall, "2026-05-05T10:00:08.000Z", "bbbbbbbb-0000-4000-8000-000000000003")).
		AddRaw(claudeErrorToolResult(
			"toolu_01Tooling00000000000000002",
			"Error: no such file or directory",
			"2026-05-05T10:00:08.500Z",
		)).
		AddRaw(claudeSidechain(
			"2026-05-05T10:00:09.000Z",
			"The backoff cap is a constant in internal/retry/config.go.",
		)).
		AddRaw(claudeReasoningOnlyAssistant(
			"2026-05-05T10:00:09.500Z",
			"Naming it from the constant rather than from memory.",
			"bbbbbbbb-0000-4000-8000-000000000005",
		)).
		AddRaw(claudeSystemRecord("2026-05-05T10:00:10.000Z")).
		AddRaw(claudeToolingUser(
			"Then name the file that holds the cap.", "2026-05-05T10:00:12.000Z")).
		AddRaw(claudeToolUse(jsonCall, "2026-05-05T10:00:12.500Z", "bbbbbbbb-0000-4000-8000-000000000006")).
		// The tool printed a JSON-encoded value, so the text the result records is itself a JSON string:
		// a conversion that decoded the already-decoded output would strip these quotation marks. This
		// result also arrives on a record the provider wrote as a system reminder -- the provider's own
		// mixed shape -- so a conversion that suppressed that record wholesale would drop this output.
		AddRaw(claudeSystemReminderToolResult(
			"toolu_01Tooling00000000000000003",
			`"maxBackoffSeconds"`,
			"2026-05-05T10:00:12.750Z",
			"bbbbbbbb-0000-4000-8000-000000000007",
			"bbbbbbbb-0000-4000-8000-000000000006",
		)).
		AddRaw(testjsonl.ClaudeAssistantJSON(
			[]map[string]any{{"type": "text", "text": "internal/retry/config.go, as maxBackoffSeconds."}},
			"2026-05-05T10:00:13.000Z",
		))
	return builder.String()
}

// claudeToolingUser is a Claude user line carrying the session's own identity and working directory.
func claudeToolingUser(text, timestamp string) string {
	return marshal(map[string]any{
		"type":      "user",
		"timestamp": timestamp,
		"version":   claudeVersion,
		"sessionId": claudeToolingSessionID,
		"cwd":       claudeToolingCwd,
		"message":   map[string]any{"content": text},
	})
}

// claudeThinkingAssistant is an assistant line whose content holds a persisted reasoning block beside
// its text, the shape a provider writes when it records thinking.
func claudeThinkingAssistant(timestamp, thinking, text, uuid string) string {
	return marshal(map[string]any{
		"type":      "assistant",
		"timestamp": timestamp,
		"uuid":      uuid,
		"message": map[string]any{
			"model": "claude-sonnet-4-5",
			"content": []map[string]any{
				{"type": "thinking", "thinking": thinking, "signature": "sig-tooling"},
				{"type": "text", "text": text},
			},
		},
	})
}

// claudeReasoningOnlyAssistant is an assistant line whose content holds a persisted reasoning block and
// nothing else: the shape a provider writes for a turn that produced no dialogue and no tool call. Its
// Content renders the trace, so a conversion that fell back to that rendering would carry the reasoning
// as the agent's own words while reporting the trace as omitted.
func claudeReasoningOnlyAssistant(timestamp, thinking, uuid string) string {
	return marshal(map[string]any{
		"type":      "assistant",
		"timestamp": timestamp,
		"uuid":      uuid,
		"message": map[string]any{
			"model": "claude-sonnet-4-5",
			"content": []map[string]any{
				{"type": "thinking", "thinking": thinking, "signature": "sig-reasoning-only"},
			},
		},
	})
}

// claudeErrorToolResult is a tool result the provider marked as an error, which is how a failed tool
// call reaches the transcript.
func claudeErrorToolResult(toolUseID, resultContent, timestamp string) string {
	return marshal(map[string]any{
		"type":      "user",
		"timestamp": timestamp,
		"message": map[string]any{
			"content": []map[string]any{{
				"type":        "tool_result",
				"tool_use_id": toolUseID,
				"content":     resultContent,
				"is_error":    true,
			}},
		},
	})
}

// claudeSystemReminderToolResult is a Claude user line the provider wrote as one record holding a system
// reminder and a tool result: the reminder is the record's whole text block, which is what classifies the
// line as system-injected, and the tool_result block answers a call an earlier line recorded. The
// inherited parser keeps both on the one message -- IsSystem set with its ToolResults populated
// (internal/parser's TestParseClaudeSession_SystemReminderPromotionPreservesToolResults) -- so a
// conversion that suppressed the record wholesale would drop a recorded tool output.
func claudeSystemReminderToolResult(toolUseID, resultContent, timestamp, uuid, parentUUID string) string {
	return marshal(map[string]any{
		"type":       "user",
		"timestamp":  timestamp,
		"uuid":       uuid,
		"parentUuid": parentUUID,
		"message": map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": "<system-reminder>the workspace state was restored</system-reminder>"},
				{"type": "tool_result", "tool_use_id": toolUseID, "content": resultContent, "is_error": false},
			},
		},
	})
}

// claudeSidechain is a subagent's turn as the provider records it in the same transcript.
func claudeSidechain(timestamp, text string) string {
	return marshal(map[string]any{
		"type":        "assistant",
		"timestamp":   timestamp,
		"isSidechain": true,
		"uuid":        "bbbbbbbb-0000-4000-8000-000000000004",
		"message": map[string]any{
			"model":   "claude-sonnet-4-5",
			"content": []map[string]any{{"type": "text", "text": text}},
		},
	})
}

// claudeSystemRecord is a provider system record: a line the provider wrote as the session's own
// bookkeeping rather than as a turn of the conversation.
func claudeSystemRecord(timestamp string) string {
	return marshal(map[string]any{
		"type":      "system",
		"subtype":   "hook_result",
		"timestamp": timestamp,
		"content":   "Hook completed.",
	})
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

// codexPatchSession is a Codex rollout whose one tool call is a patch touching two files: the shape the
// inherited reader projects into one call per file, every projection sharing the call's id and its input.
func codexPatchSession() string {
	return testjsonl.JoinJSONL(
		testjsonl.CodexSessionMetaJSON(codexPatchSessionID, codexPatchCwd, "codex_cli_rs", "2026-05-06T10:30:00.000Z"),
		testjsonl.CodexTurnContextJSON("gpt-5.2-codex", "2026-05-06T10:30:01.000Z"),
		testjsonl.CodexMsgJSON("user", "Raise the retry cap and the attempt limit in one patch.", "2026-05-06T10:30:02.000Z"),
		testjsonl.CodexFunctionCallWithCallIDJSON(
			"apply_patch",
			codexPatchCallID,
			map[string]any{"patch": codexPatchBody},
			"2026-05-06T10:30:05.000Z",
		),
		testjsonl.CodexFunctionCallOutputJSON(
			codexPatchCallID,
			"Success. Updated the following files:\nM internal/retry/config.go\nM internal/retry/policy.go",
			"2026-05-06T10:30:06.000Z",
		),
		testjsonl.CodexMsgJSON("assistant", "Both caps are patched.", "2026-05-06T10:30:07.000Z"),
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

// claudeToolUse is a Claude assistant line whose content is a tool_use block, with its own uuid when
// the transcript records one. The inherited builder covers text and usage turns; a tool_use turn is
// written here with the same native shape.
func claudeToolUse(content any, timestamp, uuid string) string {
	line := map[string]any{
		"type":      "assistant",
		"timestamp": timestamp,
		"version":   claudeVersion,
		"message": map[string]any{
			"content": content,
			"model":   "claude-sonnet-4-5",
		},
	}
	if uuid != "" {
		line["uuid"] = uuid
	}
	return marshal(line)
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
		"directory to write the fixtures into (relative to the working directory)")
	growth := flag.String("growth-root", filepath.Join("internal", "ccc", "testdata", "growth"),
		"directory to write the growth fragments into (relative to the working directory)")
	flag.Parse()

	// Both trees are written in one run, and a caller that names one root without the other gets the other
	// tree at its *default* root -- which is relative to the working directory, i.e. the committed tree
	// only when this command runs from the repository root. Every path it wrote is printed below, so a run
	// into a throw-away tree says where each tree landed.
	writeAll(*root, fixtureFiles())
	writeAll(*growth, growthFiles())
}

// writeAll writes one tree of generated files, creating the directories they need. Both trees are written
// the same way so a caller cannot generate one and forget the other: the committed bytes of the fixtures
// and of the growth fragments are both pinned against this generator.
func writeAll(root string, files map[string]string) {
	relatives := make([]string, 0, len(files))
	for relative := range files {
		relatives = append(relatives, relative)
	}
	sort.Strings(relatives)
	paths := make([]string, 0, len(files))
	for _, relative := range relatives {
		content := files[relative]
		target := filepath.Join(root, relative)
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
