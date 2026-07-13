# The hdf state machine

Every managed file on every machine is in exactly one state, derived from four
observations: the registry on `origin/main` (`.hdf/managed.toml`), the file's
blob on `origin/main`, the file's blob on this machine's branch, and (for
linking) the symlink on disk.

## States

| State        | Registry entry | Blob on origin/main | Blob on machine branch |
|--------------|----------------|---------------------|------------------------|
| Untracked    | no             | —                   | —                      |
| Enrolled     | yes            | none                | yes                    |
| Promoted     | yes            | yes                 | none                   |
| Synced       | yes            | yes (equal)         | yes (equal)            |
| Diverged     | yes            | yes (differs)       | yes (differs)          |
| RegistryOnly | yes            | none                | none                   |

`Promoted` is an observer state: another machine promoted a file this machine
has never pulled. `RegistryOnly` is reachable (a file enrolled elsewhere,
promoted nowhere) but no command currently produces outgoing transitions from
it; an `unenroll` command would need to add them.

## Transitions

| From      | Command / event                    | To       |
|-----------|------------------------------------|----------|
| Untracked | `changes-push` (enroll)            | Enrolled |
| Enrolled  | `promote`                          | Synced   |
| Promoted  | `changes-pull` → accept            | Synced   |
| Promoted  | `changes-pull` → skip              | Promoted |
| Synced    | edit + `changes-push`              | Diverged |
| Diverged  | `promote` (own history — no prompt)| Synced   |
| Diverged  | `changes-pull` → accept            | Synced   |
| Untracked | another machine promotes           | Promoted |
| Synced    | another machine re-promotes        | Diverged |

The contract table lives in `e2e_transitions_test.go` (`validTransitions`) and
every entry is exercised by `TestTransitionContractCoverage` or
`TestExternalTransitions`.

## How enrollment and promotion move content

- **`changes-push` (enroll)** commits the file and the registry to the machine
  branch, pushes the branch, and registers the file in main's registry
  *without* content. File content reaches `main` only via promote. If main's
  registry push is rejected (another machine promoted meanwhile), the entry
  rides along on this machine's next promote instead — a notice is printed.
- **`promote`** merges the machine branch into `main` with a custom two-way
  tree merge (`repo.MergeIntoBranch`): the machine branch wins per file,
  files that exist only on main are preserved, and `.hdf/managed.toml` is
  union-merged entry-by-entry (variants union per branch) so other machines'
  enrollments survive.
- **`changes-pull`** walks each registered file that differs from the machine
  branch, shows the diff, and asks per file. Accepting writes main's bytes to
  the branch (committed, registry hash recomputed from the accepted bytes);
  skipping keeps the local version. Symlinks are re-created afterwards.

## Promote guards

1. **No remote** — promote refuses outright; it is meaningless locally.
2. **Unseen-content review** — before merging, promote lists every registered
   file whose `origin/main` content has *never appeared in the machine
   branch's history* (`repo.BranchHistoryHasFileContent`). Content you
   produced or previously accepted is "seen", so the routine
   edit → `changes-push` → `promote` loop never prompts.
   - Files absent from the branch (unpulled promotes) are preserved by the
     merge; promote asks for one aggregate confirmation.
   - Diverged files this machine has never seen get a per-file diff and an
     overwrite prompt. Declining keeps main's version (per-path
     `PreferTheirs` in the merge) and is remembered in local state keyed by
     main's content hash — subsequent promotes stay non-interactive until
     main's content changes again.
   - With closed stdin (scripts), promote refuses instead of guessing.
3. **Push race** — promote fetches, fast-forwards local main to
   `origin/main`, merges, and pushes. If the main push is rejected
   non-fast-forward (another machine promoted in the window), local main is
   rolled back to `origin/main` and promote asks you to pull and retry.
   A promote interrupted between merge and push self-heals on retry.

All three commands that commit (`changes-push`, `changes-pull`, `promote`)
refuse to run when the repo has a different branch checked out than the
configured machine branch.

## Multi-machine model

Machine branches never merge main back into themselves; convergence happens
through per-file accepts and through main. Consequences:

- A machine's merge base with main advances only at its own promotes.
- The deletion guard in promote (refuse when the branch deleted a file that
  still exists on main) diffs against the merge base(s), so it only covers
  files present at the last promote point. Files acquired and deleted since
  (accepted then removed) would be resurrected by the merge — acceptable
  today because no command deletes managed files; an `unenroll` command must
  close this gap first. See the `mergeTrees` doc comment in `repo/repo.go`.

## Local state (`~/.config/hdf/state.toml`)

Daemon and CLI both mutate this file; all read-modify-write cycles go through
`config.UpdateState`, which takes an advisory file lock and saves atomically.
Fields: last sync/commit tracking, notification throttling timestamps
(including failure-notification cooldown), pending warnings, and remembered
declined overwrites.
