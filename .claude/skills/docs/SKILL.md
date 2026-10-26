---
name: docs
description: Write or review chanx-go documentation, in the voice of chanx and chanx-js. Use for Go doc comments, testable examples, doc.go, READMEs and the VitePress site in docs/, whenever a change adds or alters public API, or the user asks for docs, a guide page, an example or a docs review.
---

# Writing chanx-go docs

chanx-go has three documentation layers. Each has one job; never make two of them say the
same thing at length.

| Layer | Where | Job |
| --- | --- | --- |
| API reference | doc comments, `doc.go`, `Example*` functions, shown on pkg.go.dev | what each name does, with a runnable example |
| READMEs | one per published module, plus `examples/` | what the module is, install, one example, links on |
| Guide | the VitePress site in `docs/` | how to build something: concepts, tasks, tutorials |

The guide links an API name to pkg.go.dev instead of repeating its reference. The
reference links nowhere: it stays short and true.

## Voice

The voice of the chanx and chanx-js docs: a colleague who knows the code, explaining it to
someone smart who has not read it.

- **Lead with what the reader gets**, then how. "Every client on `/ws/chat` receives:" and
  the frame, not "The broadcast mechanism delivers messages to group members".
- **Short sentences, plain words.** One idea per sentence. "Use", not "utilize". No
  "simply", "just", "easy", "powerful", "seamless", "robust".
- **Concrete over abstract.** A real action name, a real frame, a real path:
  `{"action": "chat", "payload": {"message": "hi"}}`, `/ws/chat/{room}`.
- **Second person, present tense.** "You mount the engine", "the handler runs".
- **Show the result.** After a command or a frame, show what comes back.
- **Invite a break.** chanx-js ends its quick start with "Try breaking it": show what a
  mistake produces (a validation error frame, a panic at `Handle`), so the reader trusts
  the safety net.
- **Name the Python counterpart** when there is one (`Authenticate` is
  `authenticator_class`, `chanxtest.Spy` is `capture_broadcast_events`): many readers come
  from chanx.
- **State limits plainly.** "There is no Redis queue layer yet." Differences from Python
  go in one place, the Differences page.
- **No long dashes** (`—`). Use a colon, a comma or a new sentence. Not in code comments
  either.

## Doc comments (pkg.go.dev)

Follow [go.dev/doc/comment](https://go.dev/doc/comment) and AGENTS.md.

- Start with the name: `// Engine holds the mounted routes...`. Full sentences.
- One or two lines for most names. More only for a contract the signature cannot say:
  what panics, what is shared between connections, what is copied.
- Link names with brackets: `[Engine.Handle]`, `[layers.Layer]`, `[chanxtest.Spy]`.
- A package comment is the package's landing page. `chanx` keeps its long overview in
  `doc.go`, with `# Headings`; other packages keep one paragraph on the main file.
- Code in comments is indented by one tab after `//`.

## Testable examples

The Go way to show usage: an `Example*` function in a `_test.go` file appears on
pkg.go.dev beside the API it names, is compiled by `go vet`, and runs under `go test`.

- Name it after what it shows: `ExampleEngine_Handle`, `ExampleNewConsumer`,
  `ExampleSafeGroupName`, `Example_topics` (a suffix after `_` for a second one).
- End with `// Output:` whenever the result is deterministic, so `go test` checks it.
  Without it, the example compiles but does not run: use that for servers and handlers.
- One idea per example, 5 to 25 lines. Real names from the sandbox domain (chat, rooms,
  notifications), not `Foo`.
- An example needing `*testing.T` (chanxtest) is a whole-file example: a file holding one
  `Example` function, no `Test` functions, and the declarations it uses, such as a
  lowercase `testChat(t *testing.T)`. pkg.go.dev shows the whole file.
- Put examples in the package's external test package (`package chanx_test`) so they
  use the API as a user does.

## READMEs

Model: chanx-js's package READMEs.

1. A title, then one paragraph: what it is and the one reason to use it.
2. `go get`.
3. One example that compiles, 10 to 25 lines.
4. Short bullets or a table for what else is in the module.
5. Links on: the guide, pkg.go.dev. Then "MIT licensed."

No badges wall, no feature list longer than five bullets, no installation essay.

## Guide pages (docs/)

The site is VitePress, laid out like chanx-js's: `introduction/`, `tutorial/`, `guide/`,
`frameworks/`, `layers/`, `reference/`. The sidebar is in `docs/.vitepress/config.mts`.

- **One task or concept per page**, titled by it: "Topics", "Testing", "Sharing Redis
  with Python".
- **Open with two or three sentences**: what this is and when you need it. Then the
  smallest complete example, then the details, then edge cases and limits.
- **Code comes from compiled files.** Samples live in `docs/snippets/`, the `docs` Go
  module, tested in CI. Each snippet package has a test pinning the frames its page
  shows: write the test first, run it, then copy the frames into the page. Include a
  region with `<<< @/snippets/<dir>/<file>.go#<region>`, marking it with
  `// #region name` and `// #endregion name`. A frame, a command or a 3-line fragment may
  be inline.
- **Tables for mappings** (option to behaviour, Python to Go), prose for reasons.
- **Callouts sparingly**: `::: tip` for a shortcut, `::: warning` for a trap that loses
  data or breaks parity. At most two per page.
- **Link the next step** at the end of a page.

## Before you finish

1. `go vet ./...` and `go test ./...` in every module you touched, `docs` included:
   examples run, snippets compile and their frames hold.
2. `make docs`: dead links fail it. Check the output for a missing include too:
   VitePress renders it as text instead of failing.
3. Read each new paragraph once more as someone who has never seen chanx. Cut any
   sentence that does not help them do something.
4. Search your diff for `—`, "simply", "just", "easy" and remove them. For a new page or
   a rewrite, run the `humanizer` skill's patterns over the prose (not-X-but-Y contrasts,
   one-line closers, forced triads, bold labels), and the `diataxis` skill's check that
   a page stays one kind: tutorial, how-to, reference or explanation.
5. Behaviour you describe matches the code. When unsure, run it: the sandbox
   (`go run ./examples/cmd/sandbox`) and a WebSocket client settle it.
