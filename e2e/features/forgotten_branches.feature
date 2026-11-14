Feature: Machines and branches I forget about
  Over the years I retire machines, reinstall them, leave servers untouched
  for months, and forget to push what I changed. Each machine's branch lives
  on in the remote; hdf should keep me from losing work and let me catch up.

  Background:
    Given a remote dotfiles repository
    And a desktop machine "laptop"
    And a desktop machine "desktop"
    And a headless server "web1"
    And a headless server "nas"

  @known-gap
  Scenario: I retire a machine before promoting what I pushed from it
    # docs/state-machine.md: once the enrolling machine's branch is abandoned,
    # nothing clears a RegistryOnly entry. This pins down today's behavior; an
    # unenroll command would change it.
    Given "desktop" has a file "~/.inputrc" containing "set editing-mode vi"
    When I push "~/.inputrc" from "desktop"
    And I retire "desktop"
    Then "~/.inputrc" is RegistryOnly on "laptop"
    And "~/.inputrc" is RegistryOnly on "web1"

    When I review incoming changes on "laptop" and accept everything
    Then "~/.inputrc" is RegistryOnly on "laptop"

  Scenario: I rescue a retired machine's work by merging its branch on GitHub
    Given "desktop" has a file "~/.inputrc" containing "set editing-mode vi"
    When I push "~/.inputrc" from "desktop"
    And I retire "desktop"
    And I merge "desktop"'s branch on GitHub
    Then "~/.inputrc" is Promoted on "laptop"

    When I review incoming changes on "laptop" and accept everything
    Then "~/.inputrc" on "laptop" contains "set editing-mode vi"

  Scenario: I change files on a server but never push them
    Given "web1" has a file "~/.bashrc" containing "one"
    And "web1" has a file "~/.vimrc" containing "one"
    And "web1" has a file "~/.profile" containing "one"
    When I push "~/.bashrc" from "web1"
    And I push "~/.vimrc" from "web1"
    And I push "~/.profile" from "web1"
    And I edit "~/.bashrc" on "web1" to contain "two"
    And I edit "~/.vimrc" on "web1" to contain "two"
    And I edit "~/.profile" on "web1" to contain "two"
    And the daemon on "web1" runs one sync cycle
    Then the daemon on "web1" warned me about "uncommitted"
    And "~/.bashrc" is Enrolled on "web1"

  @bug
  Scenario: Nothing to warn about when everything is pushed
    # Bug: changes-push commits the registry entry to main but, unlike
    # promote, doesn't record main's new commit (recordMainCommit), so the
    # daemon's next cycle reports my own push as "New commits on main".
    Given "web1" has a file "~/.bashrc" containing "one"
    When I push "~/.bashrc" from "web1"
    And the daemon on "web1" runs one sync cycle
    Then the daemon on "web1" did not warn me

  Scenario: A server I haven't touched in months catches up
    Given "laptop" has a file "~/.bashrc" containing "v1"
    When I push "~/.bashrc" from "laptop"
    And I promote from "laptop"
    And I review incoming changes on "nas" and accept everything
    And I edit "~/.bashrc" on "laptop" to contain "v2"
    And I push "~/.bashrc" from "laptop"
    And I promote from "laptop"
    And I review incoming changes on "desktop" and accept everything
    And I edit "~/.bashrc" on "desktop" to contain "v3"
    And I push "~/.bashrc" from "desktop"
    And I promote from "desktop"
    Given "web1" has a file "~/.tmux.conf" containing "set -g mouse on"
    When I push "~/.tmux.conf" from "web1"
    And I promote from "web1" and continue past files I haven't pulled
    Then "~/.bashrc" is Diverged on "nas"
    And "~/.tmux.conf" is Promoted on "nas"

    When I review incoming changes on "nas" and accept everything
    Then "~/.bashrc" is Synced on "nas"
    And "~/.bashrc" on "nas" contains "v3"
    And "~/.tmux.conf" is Synced on "nas"

  Scenario: I reinstall a server and pick up its old branch
    Given "web1" has a file "~/.vimrc" containing "set number"
    When I push "~/.vimrc" from "web1"
    And I reinstall "web1" and reuse its existing branch
    Then "~/.vimrc" is Enrolled on "web1"

    When I promote from "web1"
    Then the remote main has "~/.vimrc" containing "set number"

  Scenario: I reinstall a server but start a fresh branch, forgetting the old one
    Given "web1" has a file "~/.vimrc" containing "set number"
    When I push "~/.vimrc" from "web1"
    And I reinstall "web1" and rename its existing branch
    Then "~/.vimrc" is RegistryOnly on "web1"
    And the remote main does not have "~/.vimrc"
