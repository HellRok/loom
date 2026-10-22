package tools_test

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

// Skill replay tests: each skill's tracker operations, in the order the
// operations inventory (research/skill-operations-inventory) lists them,
// mapped to tools as internal/repoinit/conventions.md maps them. Every
// step asserts the output carries what the skill needs for its next step.

// replay is the state a skill carries between steps: the values (refs,
// versions) it read from earlier outputs.
type replay struct {
	t   *testing.T
	h   *harness
	dir string
	v   map[string]any
}

// step is one tracker operation: its arguments (built from what earlier steps
// returned), and a check on what came back. wantErr names an expected error
// code instead of success.
type step struct {
	name            string
	tool            string
	args            func(r *replay) map[string]any
	wantErr         string
	bySecondSession bool // run as a second Session
	check           func(r *replay, out map[string]any)
}

func lit(m map[string]any) func(*replay) map[string]any {
	return func(*replay) map[string]any { return m }
}

func (r *replay) s(key string) string {
	r.t.Helper()
	s, ok := r.v[key].(string)
	if !ok {
		r.t.Fatalf("replay value %q = %v, want a string", key, r.v[key])
	}
	return s
}

func (r *replay) run(steps []step) {
	r.t.Helper()
	for _, st := range steps {
		args := map[string]any{}
		if st.args != nil {
			args = st.args(r)
		}
		call := r.h.call
		if st.bySecondSession {
			call = r.h.asOther
		}
		res := call(r.dir, st.tool, args)
		if st.wantErr != "" {
			if res.code() != st.wantErr {
				r.t.Fatalf("%s (%s): error = %v, want code %s", st.name, st.tool, res, st.wantErr)
			}
			if st.check != nil {
				st.check(r, res.Err)
			}
			continue
		}
		if res.Err != nil {
			r.t.Fatalf("%s (%s): unexpected error %v", st.name, st.tool, res.Err)
		}
		if st.check != nil {
			st.check(r, res.Out)
		}
	}
}

func newReplay(t *testing.T) *replay {
	h := newHarness(t)
	return &replay{t: t, h: h, dir: h.project("app"), v: map[string]any{}}
}

func mustEq(t *testing.T, what string, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s = %v, want %v", what, got, want)
	}
}

func mustURL(t *testing.T, out map[string]any, ref string) {
	t.Helper()
	want := "http://127.0.0.1:7337/app/issues/" + strings.TrimPrefix(ref, "#")
	if out["url"] != want {
		t.Fatalf("url = %v, want %v", out["url"], want)
	}
}

// created checks a create_issue output carries a ref, its URL and a version,
// and keeps the ref as v[as].
func created(as string) func(*replay, map[string]any) {
	return func(r *replay, out map[string]any) {
		ref, _ := out["ref"].(string)
		if !strings.HasPrefix(ref, "#") || out["version"] != float64(1) || out["title"] == nil {
			r.t.Fatalf("create output = %v", out)
		}
		mustURL(r.t, out, ref)
		r.v[as] = ref
		r.v[as+".url"] = out["url"]
	}
}

func asList(v any) []map[string]any {
	raw, _ := v.([]any)
	rows := make([]map[string]any, len(raw))
	for i, x := range raw {
		rows[i] = x.(map[string]any)
	}
	return rows
}

const specBody = "## Problem\n\nUsers can't log in.\n\n## Tasks\n\n- [ ] one\n- [ ] two\n\n> *quoted*\n\n```go\nfmt.Println(\"x\")\n```\n"

func TestReplayToSpec(t *testing.T) {
	r := newReplay(t)
	r.run([]step{
		{name: "publish the spec", tool: "create_issue",
			args:  lit(map[string]any{"title": "Login spec", "body": specBody}),
			check: created("spec")},
		{name: "unknown label is refused with the valid list", tool: "label", wantErr: "undefined_label",
			args: func(r *replay) map[string]any {
				return map[string]any{"ref": r.s("spec"), "add": []string{"ready-4-agent"}}
			},
			check: func(r *replay, e map[string]any) {
				if !slices.Contains(strs(e["valid"]), "ready-for-agent") {
					r.t.Fatalf("valid labels = %v", e["valid"])
				}
			}},
		{name: "apply ready-for-agent", tool: "label",
			args: func(r *replay) map[string]any {
				return map[string]any{"ref": r.s("spec"), "add": []string{"ready-for-agent"}}
			},
			check: func(r *replay, out map[string]any) {
				mustEq(r.t, "labels", strs(out["labels"]), []string{"ready-for-agent"})
				mustEq(r.t, "ref", out["ref"], r.v["spec"])
			}},
		{name: "the spec's body reads back verbatim at its URL", tool: "view_issue",
			args: func(r *replay) map[string]any { return map[string]any{"ref": r.s("spec.url")} },
			check: func(r *replay, out map[string]any) {
				mustEq(r.t, "body", out["body"], specBody)
				mustEq(r.t, "ref", out["ref"], r.v["spec"])
				mustURL(r.t, out, r.s("spec"))
			}},
	})
}

func TestReplayToTickets(t *testing.T) {
	r := newReplay(t)
	r.run([]step{
		{name: "source spec exists", tool: "create_issue",
			args: lit(map[string]any{"title": "Spec", "body": specBody}), check: created("spec")},
		{name: "comment on the spec", tool: "comment",
			args: func(r *replay) map[string]any { return map[string]any{"ref": r.s("spec"), "body": "context"} }},
		{name: "fetch the spec by its URL with body and comments", tool: "view_issue",
			args: func(r *replay) map[string]any { return map[string]any{"ref": r.s("spec.url")} },
			check: func(r *replay, out map[string]any) {
				mustEq(r.t, "body", out["body"], specBody)
				c := asList(out["comments"])
				if len(c) != 1 || c[0]["body"] != "context" || c[0]["author"] == nil || c[0]["created_at"] == nil {
					r.t.Fatalf("comments = %v", out["comments"])
				}
				r.v["specVersion"] = out["version"]
			}},
		{name: "blocker first, as a sub-issue, labelled", tool: "create_issue",
			args: func(r *replay) map[string]any {
				return map[string]any{"title": "Schema", "parent": r.s("spec"), "labels": []string{"ready-for-agent"}}
			}, check: created("a")},
		{name: "blocked child Issue references the returned ref", tool: "create_issue",
			args: func(r *replay) map[string]any {
				return map[string]any{"title": "API", "parent": r.s("spec"), "labels": []string{"ready-for-agent"}, "blocked_by": []string{r.s("a")}}
			}, check: created("b")},
		{name: "independent ticket", tool: "create_issue",
			args: func(r *replay) map[string]any {
				return map[string]any{"title": "Docs", "parent": r.s("spec"), "labels": []string{"ready-for-agent"}}
			}, check: created("c")},
		{name: "parent is not modified", tool: "view_issue",
			args: func(r *replay) map[string]any { return map[string]any{"ref": r.s("spec")} },
			check: func(r *replay, out map[string]any) {
				mustEq(r.t, "spec version", out["version"], r.v["specVersion"])
				mustEq(r.t, "spec body", out["body"], specBody)
				kids := asList(out["children"])
				mustEq(r.t, "children in order", refsOf(out["children"]), []string{r.s("a"), r.s("b"), r.s("c")})
				mustEq(r.t, "child labels", strs(kids[0]["labels"]), []string{"ready-for-agent"})
			}},
		{name: "work the frontier", tool: "frontier",
			args: func(r *replay) map[string]any { return map[string]any{"parent": r.s("spec")} },
			check: func(r *replay, out map[string]any) {
				mustEq(r.t, "frontier", refsOf(out["frontier"]), []string{r.s("a"), r.s("c")})
				mustEq(r.t, "skipped blocked", strs(out["skipped"].(map[string]any)["blocked"]), []string{r.s("b")})
			}},
		{name: "blocked-by edge is readable per ticket", tool: "view_issue",
			args: func(r *replay) map[string]any { return map[string]any{"ref": r.s("b")} },
			check: func(r *replay, out map[string]any) {
				mustEq(r.t, "blocked_by", refsOf(out["blocked_by"]), []string{r.s("a")})
				mustEq(r.t, "parent", out["parent"].(map[string]any)["ref"], r.v["spec"])
			}},
	})
}

func TestReplayTriage(t *testing.T) {
	r := newReplay(t)
	const disclaimer = "> *This was generated by AI during triage.*\n\n"
	r.run([]step{
		{name: "reporter files issues", tool: "create_issue", args: lit(map[string]any{"title": "Crash on save", "body": "It crashes."}), check: created("a")},
		{name: "second", tool: "create_issue", args: lit(map[string]any{"title": "Add dark mode"}), check: created("b")},
		{name: "third", tool: "create_issue", args: lit(map[string]any{"title": "Typo"}), check: created("c")},
		{name: "bucket 1: unlabeled, oldest first, with a count", tool: "list_issues",
			args: lit(map[string]any{"no_labels": true, "state": "open"}),
			check: func(r *replay, out map[string]any) {
				mustEq(r.t, "unlabeled", refsOf(out["issues"]), []string{r.s("a"), r.s("b"), r.s("c")})
				mustEq(r.t, "total", out["total"], float64(3))
			}},
		{name: "read the issue: body, comments, labels, author, dates", tool: "view_issue",
			args: func(r *replay) map[string]any { return map[string]any{"ref": r.s("a")} },
			check: func(r *replay, out map[string]any) {
				if out["author"] == nil || out["created_at"] == nil || out["updated_at"] == nil || out["labels"] == nil || out["comments"] == nil {
					r.t.Fatalf("view = %v", out)
				}
			}},
		{name: "category and state roles are valid labels", tool: "label",
			args: func(r *replay) map[string]any {
				return map[string]any{"ref": r.s("a"), "add": []string{"bug", "needs-triage"}}
			},
			check: func(r *replay, out map[string]any) {
				mustEq(r.t, "labels", strs(out["labels"]), []string{"bug", "needs-triage"})
			}},
		{name: "needs-info with triage notes", tool: "label",
			args: func(r *replay) map[string]any {
				return map[string]any{"ref": r.s("b"), "add": []string{"enhancement", "needs-info"}}
			}},
		{name: "triage notes comment", tool: "comment",
			args: func(r *replay) map[string]any {
				return map[string]any{"ref": r.s("b"), "body": disclaimer + "## Triage Notes\n\n@reporter which theme?"}
			},
			check: func(r *replay, out map[string]any) {
				if out["comment_id"] == nil {
					r.t.Fatalf("comment = %v", out)
				}
				mustURL(r.t, map[string]any{"url": strings.SplitN(out["url"].(string), "#", 2)[0]}, r.s("b"))
			}},
		{name: "reporter replies", tool: "comment",
			args: func(r *replay) map[string]any { return map[string]any{"ref": r.s("b"), "body": "dark"} }},
		{name: "bucket 3: needs-info comments carry author and timestamps in order", tool: "list_issues",
			args: lit(map[string]any{"labels": []string{"needs-info"}, "detail": "full"}),
			check: func(r *replay, out map[string]any) {
				rows := asList(out["issues"])
				if len(rows) != 1 || rows[0]["ref"] != r.v["b"] {
					r.t.Fatalf("needs-info = %v", rows)
				}
				cs := asList(rows[0]["comments"])
				if len(cs) != 2 || !strings.Contains(cs[0]["body"].(string), "## Triage Notes") ||
					!strings.HasPrefix(cs[0]["body"].(string), disclaimer) || cs[1]["body"] != "dark" ||
					cs[1]["author"] == nil || cs[1]["created_at"] == nil {
					r.t.Fatalf("comments = %v", cs)
				}
			}},
		{name: "bucket 2: needs-triage", tool: "list_issues",
			args: lit(map[string]any{"labels": []string{"needs-triage"}}),
			check: func(r *replay, out map[string]any) {
				mustEq(r.t, "needs-triage", refsOf(out["issues"]), []string{r.s("a")})
			}},
		{name: "agent brief comment", tool: "comment",
			args: func(r *replay) map[string]any {
				return map[string]any{"ref": r.s("a"), "body": disclaimer + "## Agent Brief\n\nFix it."}
			}},
		{name: "transition: remove old state, add new", tool: "label",
			args: func(r *replay) map[string]any {
				return map[string]any{"ref": r.s("a"), "remove": []string{"needs-triage"}, "add": []string{"ready-for-agent"}}
			},
			check: func(r *replay, out map[string]any) {
				mustEq(r.t, "labels", strs(out["labels"]), []string{"bug", "ready-for-agent"})
			}},
		{name: "what is ready for agents", tool: "list_issues",
			args: lit(map[string]any{"labels": []string{"ready-for-agent"}}),
			check: func(r *replay, out map[string]any) {
				mustEq(r.t, "ready", refsOf(out["issues"]), []string{r.s("a")})
			}},
		{name: "wontfix: comment mentioning the out-of-scope file", tool: "comment",
			args: func(r *replay) map[string]any {
				return map[string]any{"ref": r.s("c"), "body": disclaimer + "See `.out-of-scope/typo.md`."}
			}},
		{name: "wontfix: close with the wontfix label", tool: "close",
			args: func(r *replay) map[string]any {
				return map[string]any{"ref": r.s("c"), "comment": "Closing.", "wontfix": true}
			},
			check: func(r *replay, out map[string]any) {
				mustEq(r.t, "state", out["state"], "closed")
				mustEq(r.t, "already", out["already"], false)
			}},
		{name: "wontfix label is on the closed issue", tool: "view_issue",
			args: func(r *replay) map[string]any { return map[string]any{"ref": r.s("c")} },
			check: func(r *replay, out map[string]any) {
				mustEq(r.t, "state", out["state"], "closed")
				if !slices.Contains(strs(out["labels"]), "wontfix") {
					r.t.Fatalf("labels = %v", out["labels"])
				}
			}},
		{name: "unlabeled bucket is now empty", tool: "list_issues",
			args: lit(map[string]any{"no_labels": true, "state": "open"}),
			check: func(r *replay, out map[string]any) {
				mustEq(r.t, "total", out["total"], float64(0))
			}},
	})
}

func TestReplayImplement(t *testing.T) {
	r := newReplay(t)
	other := mkdir(t, t.TempDir(), "other")
	r.h.mustOK(r.h.call(other, "create_project", map[string]any{"slug": "other"}))
	r.run([]step{
		{name: "create child Issue", tool: "create_issue", args: lit(map[string]any{"title": "Add export", "body": "Do it."}), check: created("t")},
	})
	for _, form := range []string{"#1", "1", "app#1", "http://127.0.0.1:7337/app/issues/1"} {
		out := r.h.mustOK(r.h.call(r.dir, "view_issue", map[string]any{"ref": form}))
		if out["title"] != "Add export" || out["ref"] != "#1" || out["state"] != "open" {
			t.Fatalf("ref %q: view = %v", form, out)
		}
	}
	// A bare number resolves in the cwd's Project; another Project needs a slug.
	if res := r.h.call(other, "view_issue", map[string]any{"ref": "1"}); res.code() != "not_found" {
		t.Fatalf("bare 1 from other project = %v", res)
	}
	out := r.h.mustOK(r.h.call(other, "view_issue", map[string]any{"ref": "app#1"}))
	mustEq(t, "cross-project title", out["title"], "Add export")
	// An unparseable reference is a typed error the skill can ask about.
	if res := r.h.call(r.dir, "view_issue", map[string]any{"ref": "the export ticket"}); res.code() != "bad_ref" {
		t.Fatalf("garbage ref = %v", res)
	}
	// Implement never writes: the ticket is untouched.
	v := r.h.mustOK(r.h.call(r.dir, "view_issue", map[string]any{"ref": "#1", "events": true}))
	mustEq(t, "version", v["version"], float64(1))
	mustEq(t, "events", eventTypes(t, r.h, r.dir, "#1"), []string{"created"})
}

func TestReplayImplementSpec(t *testing.T) {
	r := newReplay(t)
	closeStep := func(name, key string) step {
		return step{name: name, tool: "close",
			args: func(r *replay) map[string]any {
				return map[string]any{"ref": r.s(key), "comment": "Resolved in abc123."}
			},
			check: func(r *replay, out map[string]any) {
				mustEq(r.t, "state", out["state"], "closed")
				mustEq(r.t, "ref", out["ref"], r.v[key])
			}}
	}
	frontierIs := func(name string, keys ...string) step {
		return step{name: name, tool: "frontier",
			args: func(r *replay) map[string]any { return map[string]any{"parent": r.s("spec")} },
			check: func(r *replay, out map[string]any) {
				want := []string{}
				for _, k := range keys {
					want = append(want, r.s(k))
				}
				mustEq(r.t, "frontier", refsOf(out["frontier"]), want)
			}}
	}
	r.run([]step{
		{name: "spec", tool: "create_issue", args: lit(map[string]any{"title": "Spec", "body": specBody}), check: created("spec")},
		{name: "a", tool: "create_issue", args: func(r *replay) map[string]any {
			return map[string]any{"title": "A", "parent": r.s("spec")}
		}, check: created("a")},
		{name: "b blocked by a", tool: "create_issue", args: func(r *replay) map[string]any {
			return map[string]any{"title": "B", "parent": r.s("spec"), "blocked_by": []string{r.s("a")}}
		}, check: created("b")},
		{name: "c blocked by a and b", tool: "create_issue", args: func(r *replay) map[string]any {
			return map[string]any{"title": "C", "parent": r.s("spec"), "blocked_by": []string{r.s("a"), r.s("b")}}
		}, check: created("c")},
		{name: "read the spec and its task graph", tool: "view_issue",
			args: func(r *replay) map[string]any { return map[string]any{"ref": r.s("spec")} },
			check: func(r *replay, out map[string]any) {
				mustEq(r.t, "children", refsOf(out["children"]), []string{r.s("a"), r.s("b"), r.s("c")})
			}},
		{name: "list the spec's children", tool: "list_issues",
			args: func(r *replay) map[string]any { return map[string]any{"parent": r.s("spec"), "state": "open"} },
			check: func(r *replay, out map[string]any) {
				mustEq(r.t, "children", refsOf(out["issues"]), []string{r.s("a"), r.s("b"), r.s("c")})
			}},
		{name: "list child Issues with open blockers", tool: "list_issues",
			args: func(r *replay) map[string]any { return map[string]any{"parent": r.s("spec"), "blocked": true} },
			check: func(r *replay, out map[string]any) {
				mustEq(r.t, "blocked", refsOf(out["issues"]), []string{r.s("b"), r.s("c")})
			}},
		frontierIs("frontier: only a", "a"),
		closeStep("resolve a", "a"),
		frontierIs("merge changed the frontier: only b", "b"),
		closeStep("resolve b", "b"),
		frontierIs("then c", "c"),
		closeStep("resolve c", "c"),
		frontierIs("frontier empty"),
		{name: "closed child Issue reads back with the resolution comment", tool: "view_issue",
			args: func(r *replay) map[string]any { return map[string]any{"ref": r.s("a")} },
			check: func(r *replay, out map[string]any) {
				mustEq(r.t, "state", out["state"], "closed")
				cs := asList(out["comments"])
				if len(cs) != 1 || cs[0]["body"] != "Resolved in abc123." {
					r.t.Fatalf("comments = %v", cs)
				}
			}},
		{name: "nothing open remains under the spec", tool: "list_issues",
			args: func(r *replay) map[string]any { return map[string]any{"parent": r.s("spec"), "state": "open"} },
			check: func(r *replay, out map[string]any) {
				mustEq(r.t, "total", out["total"], float64(0))
			}},
		closeStep("resolve the spec itself", "spec"),
	})
}

func TestReplayCodeReview(t *testing.T) {
	r := newReplay(t)
	other := mkdir(t, t.TempDir(), "other")
	r.h.mustOK(r.h.call(other, "create_project", map[string]any{"slug": "other"}))
	r.h.mustOK(r.h.call(other, "create_issue", map[string]any{"title": "Elsewhere", "body": "other body"}))
	r.run([]step{
		{name: "spec issue", tool: "create_issue", args: lit(map[string]any{"title": "Spec", "body": specBody}), check: created("spec")},
		{name: "commit trailer `Loom: 1` resolves", tool: "view_issue", args: lit(map[string]any{"ref": "1"}),
			check: func(r *replay, out map[string]any) {
				mustEq(r.t, "body", out["body"], specBody)
				mustEq(r.t, "title", out["title"], "Spec")
			}},
		{name: "commit trailer `Loom: other#1` resolves across Projects", tool: "view_issue", args: lit(map[string]any{"ref": "other#1"}),
			check: func(r *replay, out map[string]any) {
				mustEq(r.t, "body", out["body"], "other body")
				mustEq(r.t, "project", out["project"], "other")
			}},
		{name: "trailer for a missing issue is a clean not_found", tool: "view_issue", wantErr: "not_found", args: lit(map[string]any{"ref": "99"})},
	})
	// Code review is read-only.
	mustEq(t, "events", eventTypes(t, r.h, r.dir, "#1"), []string{"created"})
}

func TestReplayWayfinder(t *testing.T) {
	r := newReplay(t)
	r.run([]step{
		{name: "create the map", tool: "create_issue",
			args: lit(map[string]any{"kind": "map", "title": "Auth map", "body": "## Destination\n\nSSO.\n\n## Decisions so far\n\n## Fog\n\n- unknown\n"}),
			check: func(r *replay, out map[string]any) {
				mustEq(r.t, "kind", out["kind"], "map")
				created("map")(r, out)
			}},
		{name: "map is found by kind", tool: "list_issues", args: lit(map[string]any{"kind": "map"}),
			check: func(r *replay, out map[string]any) {
				mustEq(r.t, "maps", refsOf(out["issues"]), []string{r.s("map")})
			}},
		{name: "ticket without a type label is refused with valid types", tool: "create_issue", wantErr: "invalid_ticket",
			args: func(r *replay) map[string]any { return map[string]any{"title": "Q", "parent": r.s("map")} },
			check: func(r *replay, e map[string]any) {
				mustEq(r.t, "valid_types", strs(e["valid_types"]), []string{"research", "prototype", "grilling", "task"})
			}},
		{name: "ticket one", tool: "create_issue",
			args: func(r *replay) map[string]any {
				return map[string]any{"title": "Which IdP?", "parent": r.s("map"), "labels": []string{"wayfinder:research"}, "body": "## Question\n\nWhich?"}
			}, check: created("t1")},
		{name: "ticket two", tool: "create_issue",
			args: func(r *replay) map[string]any {
				return map[string]any{"title": "Session shape", "parent": r.s("map"), "labels": []string{"wayfinder:grilling"}}
			}, check: created("t2")},
		{name: "ticket three", tool: "create_issue",
			args: func(r *replay) map[string]any {
				return map[string]any{"title": "Build login", "parent": r.s("map"), "labels": []string{"wayfinder:task"}}
			}, check: created("t3")},
		{name: "second pass: wire blocking with real refs", tool: "add_blockers",
			args: func(r *replay) map[string]any {
				return map[string]any{"ref": r.s("t3"), "blocked_by": []string{r.s("t1"), r.s("t2")}}
			},
			check: func(r *replay, out map[string]any) {
				mustEq(r.t, "blocked_by", refsOf(out["blocked_by"]), []string{r.s("t1"), r.s("t2")})
			}},
		{name: "low-res map view: body plus child summaries, no child bodies", tool: "view_issue",
			args: func(r *replay) map[string]any { return map[string]any{"ref": r.s("map")} },
			check: func(r *replay, out map[string]any) {
				kids := asList(out["children"])
				mustEq(r.t, "children", refsOf(out["children"]), []string{r.s("t1"), r.s("t2"), r.s("t3")})
				for _, k := range kids {
					if _, has := k["body"]; has || k["title"] == nil || k["state"] == nil {
						r.t.Fatalf("child summary = %v", k)
					}
				}
				r.v["mapVersion"] = out["version"]
			}},
		{name: "frontier: first in map order wins", tool: "frontier",
			args: func(r *replay) map[string]any { return map[string]any{"parent": r.s("map")} },
			check: func(r *replay, out map[string]any) {
				f := asList(out["frontier"])
				mustEq(r.t, "frontier", refsOf(out["frontier"]), []string{r.s("t1"), r.s("t2")})
				// The type is read from the label, not the body.
				mustEq(r.t, "labels", strs(f[0]["labels"]), []string{"wayfinder:research"})
			}},
		{name: "claim is the first write", tool: "claim",
			args: func(r *replay) map[string]any { return map[string]any{"ref": r.s("t1")} },
			check: func(r *replay, out map[string]any) {
				mustEq(r.t, "already", out["already"], false)
				mustEq(r.t, "assignee", out["assignee"], testAssignee)
				mustEq(r.t, "holder", out["holder_session"], testSession)
			}},
		{name: "retried claim from the same session says already", tool: "claim",
			args:  func(r *replay) map[string]any { return map[string]any{"ref": r.s("t1")} },
			check: func(r *replay, out map[string]any) { mustEq(r.t, "already", out["already"], true) }},
		{name: "another session claiming the same ticket is told who holds it", tool: "claim", wantErr: "already_claimed", bySecondSession: true,
			args: func(r *replay) map[string]any { return map[string]any{"ref": r.s("t1")} }},
		{name: "claimed ticket leaves the frontier", tool: "frontier",
			args: func(r *replay) map[string]any { return map[string]any{"parent": r.s("map")} },
			check: func(r *replay, out map[string]any) {
				mustEq(r.t, "frontier", refsOf(out["frontier"]), []string{r.s("t2")})
				mustEq(r.t, "claimed", strs(out["skipped"].(map[string]any)["claimed"]), []string{r.s("t1")})
			}},
		{name: "my own claims", tool: "list_issues", args: lit(map[string]any{"held_by": "me"}),
			check: func(r *replay, out map[string]any) {
				mustEq(r.t, "held", refsOf(out["issues"]), []string{r.s("t1")})
			}},
		{name: "context pointer comment", tool: "comment",
			args: func(r *replay) map[string]any {
				return map[string]any{"ref": r.s("t1"), "body": "Findings on `research/idp`."}
			}},
		{name: "resolve: answer as a resolution comment", tool: "comment",
			args: func(r *replay) map[string]any { return map[string]any{"ref": r.s("t1"), "body": "Use Keycloak."} }},
		{name: "resolve: close", tool: "close",
			args:  func(r *replay) map[string]any { return map[string]any{"ref": r.s("t1")} },
			check: func(r *replay, out map[string]any) { mustEq(r.t, "state", out["state"], "closed") }},
		{name: "resolve: re-read for title and URL to link", tool: "view_issue",
			args: func(r *replay) map[string]any { return map[string]any{"ref": r.s("t1")} },
			check: func(r *replay, out map[string]any) {
				mustURL(r.t, out, r.s("t1"))
				r.v["pointer"] = "- [" + out["title"].(string) + "](" + out["url"].(string) + "): Keycloak."
			}},
		{name: "resolve: append the pointer to Decisions so far", tool: "append_to_section",
			args: func(r *replay) map[string]any {
				return map[string]any{"ref": r.s("map"), "heading": "Decisions so far", "text": r.s("pointer")}
			},
			check: func(r *replay, out map[string]any) {
				mustEq(r.t, "ref", out["ref"], r.v["map"])
				if out["version"] == r.v["mapVersion"] {
					r.t.Fatalf("version did not advance: %v", out)
				}
			}},
		{name: "append landed in the right section and nothing else moved", tool: "view_issue",
			args: func(r *replay) map[string]any { return map[string]any{"ref": r.s("map")} },
			check: func(r *replay, out map[string]any) {
				mustEq(r.t, "body", out["body"], "## Destination\n\nSSO.\n\n## Decisions so far\n"+r.s("pointer")+"\n\n## Fog\n\n- unknown\n")
				r.v["mapVersion"] = out["version"]
			}},
		{name: "second append is not lost", tool: "append_to_section",
			args: func(r *replay) map[string]any {
				return map[string]any{"ref": r.s("map"), "heading": "Decisions so far", "text": "- second"}
			}},
		{name: "edit with a stale version reports the current one", tool: "edit_issue", wantErr: "version_conflict",
			args: func(r *replay) map[string]any {
				return map[string]any{"ref": r.s("map"), "version": r.v["mapVersion"], "body": "replaced"}
			},
			check: func(r *replay, e map[string]any) {
				cur := e["current"].(map[string]any)
				if !strings.Contains(cur["body"].(string), "- second") || cur["version"] == r.v["mapVersion"] {
					r.t.Fatalf("current = %v", cur)
				}
				r.v["mapVersion"] = cur["version"]
			}},
		{name: "edit with the current version succeeds (Out of scope / Fog edits)", tool: "edit_issue",
			args: func(r *replay) map[string]any {
				return map[string]any{"ref": r.s("map"), "version": r.v["mapVersion"], "title": "Auth map v2"}
			},
			check: func(r *replay, out map[string]any) { mustEq(r.t, "ref", out["ref"], r.v["map"]) }},
		{name: "unblocking: t3 still blocked by t2", tool: "frontier",
			args: func(r *replay) map[string]any { return map[string]any{"parent": r.s("map")} },
			check: func(r *replay, out map[string]any) {
				mustEq(r.t, "frontier", refsOf(out["frontier"]), []string{r.s("t2")})
			}},
		{name: "out of scope: close the ticket", tool: "close",
			args: func(r *replay) map[string]any {
				return map[string]any{"ref": r.s("t2"), "comment": "Out of scope."}
			}},
		{name: "ticket became unblocked", tool: "frontier",
			args: func(r *replay) map[string]any { return map[string]any{"parent": r.s("map")} },
			check: func(r *replay, out map[string]any) {
				mustEq(r.t, "frontier", refsOf(out["frontier"]), []string{r.s("t3")})
			}},
		{name: "decision invalidates a ticket: a wrong confirmation is refused", tool: "delete_issue", wantErr: "confirm_mismatch",
			args: func(r *replay) map[string]any { return map[string]any{"ref": r.s("t3"), "confirm": "#999"} }},
		{name: "delete with confirmation", tool: "delete_issue",
			args: func(r *replay) map[string]any { return map[string]any{"ref": r.s("t3"), "confirm": r.s("t3")} }},
		{name: "deleted ticket is gone", tool: "view_issue", wantErr: "not_found",
			args: func(r *replay) map[string]any { return map[string]any{"ref": r.s("t3")} }},
		{name: "closed child Issues stay viewable on demand", tool: "view_issue",
			args: func(r *replay) map[string]any { return map[string]any{"ref": r.s("t1")} },
			check: func(r *replay, out map[string]any) {
				mustEq(r.t, "state", out["state"], "closed")
				cs := asList(out["comments"])
				mustEq(r.t, "last comment", cs[len(cs)-1]["body"], "Use Keycloak.")
			}},
	})
}

func TestReplayAskMatt(t *testing.T) {
	// ask-matt is a router with no tracker calls of its own; it promises
	// native blocking, a frontier, and that to-tickets output skips triage.
	r := newReplay(t)
	r.run([]step{
		{name: "spec", tool: "create_issue", args: lit(map[string]any{"title": "Spec"}), check: created("spec")},
		{name: "ticket", tool: "create_issue", args: func(r *replay) map[string]any {
			return map[string]any{"title": "A", "parent": r.s("spec"), "labels": []string{"ready-for-agent"}}
		}, check: created("a")},
		{name: "blocked child Issue", tool: "create_issue", args: func(r *replay) map[string]any {
			return map[string]any{"title": "B", "parent": r.s("spec"), "labels": []string{"ready-for-agent"}, "blocked_by": []string{r.s("a")}}
		}, check: created("b")},
		{name: "a child Issue whose blockers are done can be grabbed", tool: "close",
			args: func(r *replay) map[string]any { return map[string]any{"ref": r.s("a")} }},
		{name: "frontier across the spec", tool: "frontier",
			args: func(r *replay) map[string]any { return map[string]any{"parent": r.s("spec")} },
			check: func(r *replay, out map[string]any) {
				mustEq(r.t, "frontier", refsOf(out["frontier"]), []string{r.s("b")})
			}},
		{name: "to-tickets output is already labelled, so triage's unlabeled bucket skips it", tool: "list_issues",
			args: lit(map[string]any{"no_labels": true}),
			check: func(r *replay, out map[string]any) {
				mustEq(r.t, "unlabeled", refsOf(out["issues"]), []string{r.s("spec")})
			}},
	})
}
