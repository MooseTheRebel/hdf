Feature: Everyday syncing across my machines
  I keep my dotfiles in one repository and use them on several machines:
  two desktops (with the GUI) and two headless servers (CLI only). Each
  machine works on its own branch; a change reaches my other machines once I
  promote it to main and then review it on each of them.

  Background:
    Given a remote dotfiles repository
    And a desktop machine "laptop"
    And a desktop machine "desktop"
    And a headless server "web1"
    And a headless server "nas"

  Scenario: A change made on my laptop reaches every machine
    When I edit "~/.bashrc" on "laptop" to contain "alias ll='ls -l'"
    And I push "~/.bashrc" from "laptop"
    Then "~/.bashrc" is Enrolled on "laptop"
    And "~/.bashrc" is RegistryOnly on "web1"
    And the remote main does not have "~/.bashrc"

    When I promote from "laptop"
    Then "~/.bashrc" is Synced on "laptop"
    And "~/.bashrc" is Promoted on "desktop"
    And the remote main has "~/.bashrc" containing "alias ll='ls -l'"

    When I review incoming changes on "desktop" and accept everything
    And I review incoming changes on "web1" and accept "~/.bashrc"
    Then "~/.bashrc" is Synced on "desktop"
    And "~/.bashrc" is Synced on "web1"
    And "~/.bashrc" on "web1" contains "alias ll='ls -l'"
    And "~/.bashrc" is Promoted on "nas"

    When I review incoming changes on "nas" and accept everything
    Then "~/.bashrc" is Synced on "nas"
    And "~/.bashrc" on "nas" contains "alias ll='ls -l'"

  Scenario Outline: Each machine adds its own file, in any order, and everyone converges
    When I edit "<file>" on "<machine>" to contain "<content>"
    And I push "<file>" from "<machine>"
    And I promote from "<machine>"
    And I review incoming changes on "laptop" and accept everything
    And I review incoming changes on "desktop" and accept everything
    And I review incoming changes on "web1" and accept everything
    And I review incoming changes on "nas" and accept everything
    Then "<file>" is Synced on "laptop"
    And "<file>" is Synced on "desktop"
    And "<file>" is Synced on "web1"
    And "<file>" is Synced on "nas"

    Examples:
      | machine | file          | content         |
      | laptop  | ~/.vimrc      | set number      |
      | web1    | ~/.tmux.conf  | set -g mouse on |
      | nas     | ~/.profile    | export EDITOR=vi |

  Scenario: Updating a file everyone already has
    Given "laptop" has a file "~/.gitconfig" containing "[user] name = me"
    When I push "~/.gitconfig" from "laptop"
    And I promote from "laptop"
    And I review incoming changes on "web1" and accept everything
    Then "~/.gitconfig" is Synced on "web1"

    When I edit "~/.gitconfig" on "web1" to contain "[user] name = me, on a server"
    And I push "~/.gitconfig" from "web1"
    Then "~/.gitconfig" is Diverged on "web1"
    And "~/.gitconfig" is Synced on "laptop"

    When I promote from "web1"
    Then "~/.gitconfig" is Synced on "web1"
    And "~/.gitconfig" is Diverged on "laptop"

    When I review incoming changes on "laptop" and accept "~/.gitconfig"
    Then "~/.gitconfig" is Synced on "laptop"
    And "~/.gitconfig" on "laptop" contains "[user] name = me, on a server"

  Scenario: I'm not ready for a new file on a server yet
    Given "laptop" has a file "~/.zshrc" containing "setopt autocd"
    When I push "~/.zshrc" from "laptop"
    And I promote from "laptop"
    And I review incoming changes on "nas" and skip "~/.zshrc"
    Then "~/.zshrc" is Promoted on "nas"

    When I review incoming changes on "nas" and accept "~/.zshrc"
    Then "~/.zshrc" is Synced on "nas"
    And "~/.zshrc" on "nas" contains "setopt autocd"

  @gui-pending
  Scenario: I review incoming changes on my desktop in the GUI
    # Runs over the CLI until the GUI's code path (cli.App) is driven directly.
    Given "web1" has a file "~/.tmux.conf" containing "set -g mouse on"
    When I push "~/.tmux.conf" from "web1"
    And I promote from "web1"
    And I review incoming changes on "desktop" using the GUI and accept everything
    Then "~/.tmux.conf" is Synced on "desktop"
