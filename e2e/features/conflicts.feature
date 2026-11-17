Feature: Conflicting changes between my machines
  When I change the same file on two machines, or promote from a machine
  that hasn't seen my latest work, hdf must never silently throw a change
  away: it shows me what's different and lets me decide.

  Background:
    Given a remote dotfiles repository
    And a desktop machine "laptop"
    And a headless server "web1"
    And a headless server "nas"

  Scenario: I start the same file on two machines before either is promoted
    Given "laptop" has a file "~/.bashrc" containing "export PS1='laptop$ '"
    And "web1" has a file "~/.bashrc" containing "export PS1='web1$ '"
    When I push "~/.bashrc" from "laptop"
    And I push "~/.bashrc" from "web1"
    And I promote from "laptop"
    Then "~/.bashrc" is Synced on "laptop"
    And "~/.bashrc" is Diverged on "web1"

    When I review incoming changes on "web1" and skip "~/.bashrc"
    Then "~/.bashrc" is Diverged on "web1"
    And "~/.bashrc" on "web1" contains "export PS1='web1$ '"

    When I review incoming changes on "web1" and accept "~/.bashrc"
    Then "~/.bashrc" is Synced on "web1"
    And "~/.bashrc" on "web1" contains "export PS1='laptop$ '"

  Scenario: A machine that hasn't pulled my latest change can't silently undo it
    Given "laptop" has a file "~/.bashrc" containing "v1"
    When I push "~/.bashrc" from "laptop"
    And I promote from "laptop"
    And I review incoming changes on "web1" and accept everything
    And I review incoming changes on "nas" and accept everything
    And I edit "~/.bashrc" on "laptop" to contain "v2"
    And I push "~/.bashrc" from "laptop"
    And I promote from "laptop"
    Then "~/.bashrc" is Diverged on "web1"

    # web1 still has v1 and promotes unrelated work.
    Given "web1" has a file "~/.vimrc" containing "set number"
    When I push "~/.vimrc" from "web1"
    And I try to promote from "web1" without a terminal
    Then the last command on "web1" failed mentioning "haven't reviewed"
    And the remote main has "~/.bashrc" containing "v2"

    When I promote from "web1" and keep main's newer "~/.bashrc"
    Then the remote main has "~/.bashrc" containing "v2"
    And the remote main has "~/.vimrc" containing "set number"
    And "~/.bashrc" is Synced on "laptop"
    And "~/.bashrc" is Diverged on "web1"
    And "~/.vimrc" is Promoted on "nas"

  Scenario: I deliberately overwrite main's newer version from a stale machine
    Given "laptop" has a file "~/.bashrc" containing "v1"
    When I push "~/.bashrc" from "laptop"
    And I promote from "laptop"
    And I review incoming changes on "web1" and accept everything
    And I edit "~/.bashrc" on "laptop" to contain "v2"
    And I push "~/.bashrc" from "laptop"
    And I promote from "laptop"
    And I promote from "web1" and overwrite main's newer "~/.bashrc"
    Then the remote main has "~/.bashrc" containing "v1"
    And "~/.bashrc" is Synced on "web1"
    And "~/.bashrc" is Diverged on "laptop"

  Scenario: Promoting while another machine's file is still waiting for me
    Given "laptop" has a file "~/.bashrc" containing "from laptop"
    When I push "~/.bashrc" from "laptop"
    And I promote from "laptop"
    Given "web1" has a file "~/.vimrc" containing "set wrap"
    When I push "~/.vimrc" from "web1"
    And I try to promote from "web1" without a terminal
    Then the last command on "web1" failed mentioning "changes-pull"

    When I promote from "web1" and continue past files I haven't pulled
    Then the remote main has "~/.bashrc" containing "from laptop"
    And the remote main has "~/.vimrc" containing "set wrap"
    And "~/.bashrc" is Promoted on "web1"
    And "~/.vimrc" is Synced on "web1"

  Scenario: I merge a machine's branch on GitHub instead of promoting
    Given "nas" has a file "~/.profile" containing "export EDITOR=vi"
    When I push "~/.profile" from "nas"
    And I merge "nas"'s branch on GitHub
    Then the remote main has "~/.profile" containing "export EDITOR=vi"
    And "~/.profile" is Synced on "nas"
    And "~/.profile" is Promoted on "laptop"

    When I review incoming changes on "laptop" and accept everything
    Then "~/.profile" is Synced on "laptop"

  Scenario: A GitHub merge and a CLI promote land close together
    Given "nas" has a file "~/.profile" containing "export EDITOR=vi"
    And "laptop" has a file "~/.bashrc" containing "alias g=git"
    When I push "~/.profile" from "nas"
    And I push "~/.bashrc" from "laptop"
    And I merge "nas"'s branch on GitHub
    And I promote from "laptop" and continue past files I haven't pulled
    Then the remote main has "~/.profile" containing "export EDITOR=vi"
    And the remote main has "~/.bashrc" containing "alias g=git"
    When I review incoming changes on "web1" and accept everything
    Then "~/.profile" is Synced on "web1"
    And "~/.bashrc" is Synced on "web1"

  Scenario: I review what's waiting, then promote
    Given "laptop" has a file "~/.bashrc" containing "from laptop"
    When I push "~/.bashrc" from "laptop"
    And I promote from "laptop"
    Given "web1" has a file "~/.vimrc" containing "set wrap"
    When I push "~/.vimrc" from "web1"
    And I try to promote from "web1" without a terminal
    Then the last command on "web1" failed mentioning "changes-pull"

    When I review incoming changes on "web1" and accept "~/.bashrc"
    And I promote from "web1"
    Then "~/.bashrc" is Synced on "web1"
    And "~/.vimrc" is Synced on "web1"
    And "~/.vimrc" is Promoted on "laptop"
