Feature: Working while a machine can't reach the remote
  Machines lose their network: a laptop on a plane, a server behind a
  broken VPN. Work done there must not be lost, must not reach my other
  machines until it's actually pushed, and must catch up once the machine
  is back online.

  Background:
    Given a remote dotfiles repository
    And a desktop machine "laptop"
    And a headless server "web1"

  Scenario: A change made offline on a server waits until the server is back
    Given "web1" is offline
    And "web1" has a file "~/.bashrc" containing "set -o vi"
    When I try to push "~/.bashrc" from "web1"
    Then the last command on "web1" failed
    And "web1"'s branch on the remote is missing the local branch
    And "~/.bashrc" is Untracked on "laptop"

    When I try to promote from "web1"
    Then the last command on "web1" failed
    And the remote main does not have "~/.bashrc"

    When "web1" comes back online
    And the daemon on "web1" runs one sync cycle
    Then "web1"'s branch on the remote matches its local branch

    When I promote from "web1"
    Then the remote main has "~/.bashrc" containing "set -o vi"
    And "~/.bashrc" is Synced on "web1"
    And "~/.bashrc" is Promoted on "laptop"

  @bug
  Scenario: Pushing again by hand after coming back online
    # Bug: the retry stops at "already managed and unchanged" (runEnroll)
    # before pushing, so the branch stays unpushed until the daemon runs.
    Given "laptop" is offline
    And "laptop" has a file "~/.vimrc" containing "set number"
    When I try to push "~/.vimrc" from "laptop"
    And "laptop" comes back online
    And I push "~/.vimrc" from "laptop"
    Then "laptop"'s branch on the remote matches its local branch

  Scenario: The daemon tells me when it can't reach the remote
    Given "laptop" has a file "~/.bashrc" containing "alias ll='ls -l'"
    When I push "~/.bashrc" from "laptop"
    And "laptop" goes offline
    And the daemon on "laptop" runs one sync cycle
    Then the daemon on "laptop" warned me about "fetch from remote failed"

    When "laptop" comes back online
    And I acknowledge the daemon's warnings on "laptop"
    And I promote from "laptop"
    Then the remote main has "~/.bashrc" containing "alias ll='ls -l'"

  Scenario: Another machine keeps working while one is offline
    Given "laptop" has a file "~/.bashrc" containing "v1"
    When I push "~/.bashrc" from "laptop"
    And I promote from "laptop"
    And "web1" goes offline
    And I edit "~/.bashrc" on "laptop" to contain "v2"
    And I push "~/.bashrc" from "laptop"
    And I promote from "laptop"
    And I try to review incoming changes on "web1" and accept everything
    Then "~/.bashrc" is Promoted on "web1"

    When "web1" comes back online
    And I review incoming changes on "web1" and accept everything
    Then "~/.bashrc" is Synced on "web1"
    And "~/.bashrc" on "web1" contains "v2"
