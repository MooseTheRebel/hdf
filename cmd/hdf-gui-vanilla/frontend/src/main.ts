import './style.css';
import './app.css';

import {IsInitialized, HasDiff, GetDiffContent, GetCurrentIndex, GetTotalDiffs, NextDiff, PreviousDiff, CloseWindow, GetStatus, GetConfig, GetDaemonStatus, InstallDaemon, UninstallDaemon, StartDaemon, StopDaemon, GetPendingWarnings, StartLink, AcceptIncomingFile, FinishLink, PickFileToEnroll, StartEnroll, ConfirmEnroll, DefaultRepoPath, PickDirectory, StartInitLocal, StartInitRemote, ResolveBranchCollision, FinishInit, StartPromote, ResolveDivergedFile, FinishPromote, SubmitReportIssue} from '../wailsjs/go/cli/App';
import type {cli} from '../wailsjs/go/models';
import {renderDiffContent} from './diff';
import {renderStatus} from './status';
import {renderConfig} from './configinfo';
import {renderDaemonStatus} from './daemonstatus';
import {renderDaemonManagement} from './daemonmanagement';
import {renderPendingWarnings, renderIncomingFileReview, renderLinkResults} from './linkflow';
import {renderEnrollPreview, renderEnrollResult} from './enrollflow';
import {renderLocalForm, renderRemoteForm, renderBranchCollision, renderInitResult} from './initflow';
import {renderPreservedFiles, renderDivergedFileReview, renderPromoteResult} from './promoteflow';
import {renderReportIssueResult} from './reportissueflow';
import {fmt, msg} from '@locales';
import {html} from './i18n';

HasDiff().then((hasDiff) => {
    if (hasDiff) {
        displayDiffViewer();
    } else {
        displayHomeScreen();
    }
}).catch(() => {
    displayHomeScreen();
});

function displayHomeScreen() {
    const app = document.querySelector('#app');
    if (!app) return;
    IsInitialized().then((initialized) => {
        if (initialized) {
            const c = msg.home.commands;
            app.innerHTML = `
                <div class="home-container">
                    ${homeHeader(msg.home.badgeInitialized, 'initialized')}
                    <p class="home-subtitle">${html(msg.home.subtitleInitialized)}</p>
                    <div class="command-list">
                        ${commandRow(c.enroll, 'enroll-btn')}
                        ${commandRow(c.link, 'link-btn')}
                        ${commandRow(c.linkNoFetch, 'link-no-fetch-btn')}
                        ${commandRow(c.promote, 'promote-btn')}
                        ${commandRow(c.status, 'status-btn')}
                        ${commandRow(c.daemon)}
                        ${commandRow(c.diff)}
                        ${commandRow(c.config, 'config-btn')}
                        ${commandRow(c.daemonStatus, 'daemon-status-btn')}
                        ${commandRow(c.daemonManagement, 'daemon-management-btn')}
                        ${commandRow(c.reportIssue, 'report-issue-btn')}
                    </div>
                    <button class="close-button" id="close-btn">${html(msg.common.close)}</button>
                </div>
            `;
        } else {
            const steps = msg.home.steps.map((step, i) => `
                        <div class="step">
                            <span class="step-number">${i + 1}</span>
                            <div class="step-body">
                                <div class="step-label">${html(step.label)}</div>
                                <code class="step-cmd">${html(step.command)}</code>
                                <div class="step-hint">${html(step.hint)}</div>
                            </div>
                        </div>`).join('');
            app.innerHTML = `
                <div class="home-container">
                    ${homeHeader(msg.home.badgeNotInitialized, 'not-initialized')}
                    <p class="home-subtitle">${html(msg.home.subtitleNotInitialized)}</p>
                    <div class="steps">${steps}
                    </div>
                    <button class="control-btn" id="get-started-btn">${html(msg.home.getStarted)}</button>
                    <button class="close-button" id="close-btn">${html(msg.common.close)}</button>
                </div>
            `;
        }

        document.getElementById('close-btn')?.addEventListener('click', () => CloseWindow());
        document.getElementById('get-started-btn')?.addEventListener('click', () => displayInitModeSelect());
        document.getElementById('enroll-btn')?.addEventListener('click', () => startEnrollFlow());
        document.getElementById('link-btn')?.addEventListener('click', () => displayLinkView(false));
        document.getElementById('link-no-fetch-btn')?.addEventListener('click', () => displayLinkView(true));
        document.getElementById('promote-btn')?.addEventListener('click', () => displayPromoteView());
        document.getElementById('status-btn')?.addEventListener('click', () => displayStatusView());
        document.getElementById('config-btn')?.addEventListener('click', () => displayConfigView());
        document.getElementById('daemon-status-btn')?.addEventListener('click', () => displayDaemonStatusView());
        document.getElementById('daemon-management-btn')?.addEventListener('click', () => displayDaemonManagementView());
        document.getElementById('report-issue-btn')?.addEventListener('click', () => displayReportIssueView());
    }).catch((err) => {
        app.innerHTML = `
            <div class="home-container">
                ${homeHeader(msg.home.badgeError, 'not-initialized')}
                <p class="home-subtitle">${html(msg.home.configError)}</p>
                <p class="home-subtitle" id="error-message"></p>
                <button class="close-button" id="error-close-btn">${html(msg.common.close)}</button>
            </div>
        `;
        const errorMsgEl = document.getElementById('error-message');
        if (errorMsgEl) errorMsgEl.textContent = String(err);
        document.getElementById('error-close-btn')?.addEventListener('click', () => CloseWindow());
    });
}

function homeHeader(badge: string, badgeClass: string): string {
    return `
        <div class="home-header">
            <h1 class="home-title">${html(msg.home.title)}</h1>
            <span class="home-badge ${badgeClass}">${html(badge)}</span>
        </div>
    `;
}

// commandRow renders a home-screen command: a button when it has an id
// (clickable), otherwise a plain row.
function commandRow(command: {command: string; description: string}, id?: string): string {
    const body = `
        <code class="cmd">${html(command.command)}</code>
        <span class="cmd-desc">${html(command.description)}</span>`;
    return id
        ? `<button class="command-row command-row-clickable" id="${id}">${body}</button>`
        : `<div class="command-row">${body}</div>`;
}

function displayStatusView() {
    const app = document.querySelector('#app');
    if (!app) return;
    app.innerHTML = `
        <div class="status-container">
            <div class="status-header-section">
                <h1>${html(msg.status.title)}</h1>
            </div>
            <div id="status-loading">${html(msg.status.loading)}</div>
            <div id="status-content" style="display: none;"></div>
            <div class="status-controls">
                <button id="status-back-btn" class="control-btn">${html(msg.common.back)}</button>
            </div>
        </div>
    `;
    document.getElementById('status-back-btn')?.addEventListener('click', () => displayHomeScreen());

    GetStatus().then((info) => {
        const loadingEl = document.getElementById('status-loading');
        const contentEl = document.getElementById('status-content');
        if (loadingEl) loadingEl.style.display = 'none';
        if (contentEl) {
            contentEl.innerHTML = renderStatus(info);
            contentEl.style.display = 'block';
        }
    }).catch((err) => {
        const loadingEl = document.getElementById('status-loading');
        if (loadingEl) loadingEl.textContent = fmt(msg.status.loadError, {error: String(err)});
    });
}

function displayConfigView() {
    const app = document.querySelector('#app');
    if (!app) return;
    app.innerHTML = `
        <div class="config-container">
            <div class="config-header-section">
                <h1>${html(msg.config.title)}</h1>
            </div>
            <div id="config-loading">${html(msg.config.loading)}</div>
            <div id="config-content" style="display: none;"></div>
            <div class="config-controls">
                <button id="config-back-btn" class="control-btn">${html(msg.common.back)}</button>
            </div>
        </div>
    `;
    document.getElementById('config-back-btn')?.addEventListener('click', () => displayHomeScreen());

    GetConfig().then((info) => {
        const loadingEl = document.getElementById('config-loading');
        const contentEl = document.getElementById('config-content');
        if (loadingEl) loadingEl.style.display = 'none';
        if (contentEl) {
            contentEl.innerHTML = renderConfig(info);
            contentEl.style.display = 'block';
        }
    }).catch((err) => {
        const loadingEl = document.getElementById('config-loading');
        if (loadingEl) loadingEl.textContent = fmt(msg.config.loadError, {error: String(err)});
    });
}

function displayDaemonStatusView() {
    const app = document.querySelector('#app');
    if (!app) return;
    app.innerHTML = `
        <div class="daemon-status-container">
            <div class="daemon-status-header-section">
                <h1>${html(msg.daemon.statusTitle)}</h1>
            </div>
            <div id="daemon-status-loading">${html(msg.daemon.loading)}</div>
            <div id="daemon-status-content" style="display: none;"></div>
            <div class="daemon-status-controls">
                <button id="daemon-status-back-btn" class="control-btn">${html(msg.common.back)}</button>
            </div>
        </div>
    `;
    document.getElementById('daemon-status-back-btn')?.addEventListener('click', () => displayHomeScreen());

    GetDaemonStatus().then((status) => {
        const loadingEl = document.getElementById('daemon-status-loading');
        const contentEl = document.getElementById('daemon-status-content');
        if (loadingEl) loadingEl.style.display = 'none';
        if (contentEl) {
            contentEl.innerHTML = renderDaemonStatus(status);
            contentEl.style.display = 'block';
        }
    }).catch((err) => {
        const loadingEl = document.getElementById('daemon-status-loading');
        if (loadingEl) loadingEl.textContent = fmt(msg.daemon.loadError, {error: String(err)});
    });
}

function displayDaemonManagementView() {
    const app = document.querySelector('#app');
    if (!app) return;
    app.innerHTML = `
        <div class="daemon-management-container">
            <div class="daemon-management-header-section">
                <h1>${html(msg.daemon.managementTitle)}</h1>
            </div>
            <div id="daemon-management-loading">${html(msg.daemon.loading)}</div>
            <div id="daemon-management-content" style="display: none;"></div>
            <div id="daemon-management-result"></div>
            <div class="daemon-management-controls">
                <button id="daemon-management-back-btn" class="control-btn">${html(msg.common.back)}</button>
            </div>
        </div>
    `;
    document.getElementById('daemon-management-back-btn')?.addEventListener('click', () => displayHomeScreen());

    refreshDaemonManagementStatus();
}

function refreshDaemonManagementStatus() {
    const loadingEl = document.getElementById('daemon-management-loading');
    const contentEl = document.getElementById('daemon-management-content');
    if (loadingEl) loadingEl.style.display = 'block';
    if (contentEl) contentEl.style.display = 'none';

    GetDaemonStatus().then((status) => {
        if (loadingEl) loadingEl.style.display = 'none';
        if (contentEl) {
            contentEl.innerHTML = renderDaemonManagement(status);
            contentEl.style.display = 'block';
        }
        wireDaemonManagementActions();
    }).catch((err) => {
        if (loadingEl) loadingEl.textContent = fmt(msg.daemon.loadError, {error: String(err)});
    });
}

function wireDaemonManagementActions() {
    const resultEl = document.getElementById('daemon-management-result');
    const runAction = (action: () => Promise<void>, successMessage: string) => {
        if (resultEl) resultEl.textContent = '';
        action().then(() => {
            if (resultEl) resultEl.textContent = successMessage;
            refreshDaemonManagementStatus();
        }).catch((err) => {
            if (resultEl) resultEl.textContent = fmt(msg.daemon.actionError, {error: String(err)});
        });
    };

    document.getElementById('daemon-install-btn')?.addEventListener('click', () => runAction(InstallDaemon, msg.daemon.installed));
    document.getElementById('daemon-uninstall-btn')?.addEventListener('click', () => runAction(UninstallDaemon, msg.daemon.uninstalled));
    document.getElementById('daemon-start-btn')?.addEventListener('click', () => runAction(StartDaemon, msg.daemon.started));
    document.getElementById('daemon-stop-btn')?.addEventListener('click', () => runAction(StopDaemon, msg.daemon.stopped));
}

function startEnrollFlow() {
    PickFileToEnroll().then((path) => {
        if (!path) return;
        displayEnrollView(path);
    }).catch((err) => {
        displayEnrollErrorScreen(fmt(msg.enroll.pickError, {error: String(err)}));
    });
}

function displayEnrollView(path: string) {
    const app = document.querySelector('#app');
    if (!app) return;
    app.innerHTML = `
        <div class="link-container">
            <div class="link-header-section">
                <h1>${html(msg.enroll.title)}</h1>
            </div>
            <div id="enroll-step-content">${html(msg.warnings.checking)}</div>
            <div class="link-controls" id="enroll-controls"></div>
        </div>
    `;
    runEnrollWarningsStep(path);
}

function displayEnrollErrorScreen(message: string) {
    const app = document.querySelector('#app');
    if (!app) return;
    app.innerHTML = `
        <div class="link-container">
            <div class="link-header-section">
                <h1>${html(msg.enroll.title)}</h1>
            </div>
            <div id="enroll-step-content"></div>
            <div class="link-controls" id="enroll-controls"></div>
        </div>
    `;
    showEnrollError(message);
}

function runEnrollWarningsStep(path: string) {
    GetPendingWarnings().then((warnings) => {
        if (warnings.length > 0) {
            showEnrollWarnings(warnings, path);
        } else {
            runEnrollStartStep(path);
        }
    }).catch((err) => {
        showEnrollError(fmt(msg.warnings.checkError, {error: String(err)}));
    });
}

function showEnrollWarnings(warnings: string[], path: string) {
    const contentEl = document.getElementById('enroll-step-content');
    const controlsEl = document.getElementById('enroll-controls');
    if (contentEl) contentEl.innerHTML = renderPendingWarnings(warnings);
    if (controlsEl) {
        controlsEl.innerHTML = `
            <button id="enroll-warnings-continue-btn" class="control-btn">${html(msg.common.continue)}</button>
            <button id="enroll-warnings-cancel-btn" class="control-btn">${html(msg.common.cancel)}</button>
        `;
    }
    document.getElementById('enroll-warnings-continue-btn')?.addEventListener('click', () => runEnrollStartStep(path));
    document.getElementById('enroll-warnings-cancel-btn')?.addEventListener('click', () => displayHomeScreen());
}

function runEnrollStartStep(path: string) {
    const contentEl = document.getElementById('enroll-step-content');
    const controlsEl = document.getElementById('enroll-controls');
    if (contentEl) contentEl.textContent = msg.enroll.preparing;
    if (controlsEl) controlsEl.innerHTML = '';

    StartEnroll(path).then((info) => {
        if (contentEl) contentEl.innerHTML = renderEnrollPreview(info);
        if (controlsEl) {
            controlsEl.innerHTML = `
                <button id="enroll-confirm-btn" class="control-btn">${html(msg.enroll.confirm)}</button>
                <button id="enroll-cancel-btn" class="control-btn">${html(msg.common.cancel)}</button>
            `;
        }
        document.getElementById('enroll-confirm-btn')?.addEventListener('click', () => runEnrollConfirmStep());
        document.getElementById('enroll-cancel-btn')?.addEventListener('click', () => displayHomeScreen());
    }).catch((err) => {
        showEnrollError(fmt(msg.enroll.startError, {error: String(err)}));
    });
}

function runEnrollConfirmStep() {
    const contentEl = document.getElementById('enroll-step-content');
    const controlsEl = document.getElementById('enroll-controls');
    if (contentEl) contentEl.textContent = msg.enroll.enrolling;
    if (controlsEl) controlsEl.innerHTML = '';

    ConfirmEnroll().then((result) => {
        if (contentEl) contentEl.innerHTML = renderEnrollResult(result);
        if (controlsEl) controlsEl.innerHTML = '<button id="enroll-done-back-btn" class="control-btn">${html(msg.common.back)}</button>';
        document.getElementById('enroll-done-back-btn')?.addEventListener('click', () => displayHomeScreen());
    }).catch((err) => {
        showEnrollError(fmt(msg.enroll.error, {error: String(err)}));
    });
}

function showEnrollError(message: string) {
    const contentEl = document.getElementById('enroll-step-content');
    const controlsEl = document.getElementById('enroll-controls');
    if (contentEl) contentEl.textContent = message;
    if (controlsEl) controlsEl.innerHTML = '<button id="enroll-error-back-btn" class="control-btn">${html(msg.common.back)}</button>';
    document.getElementById('enroll-error-back-btn')?.addEventListener('click', () => displayHomeScreen());
}

function displayInitModeSelect() {
    const app = document.querySelector('#app');
    if (!app) return;
    app.innerHTML = `
        <div class="link-container">
            <div class="link-header-section">
                <h1>${html(msg.init.title)}</h1>
            </div>
            <div id="init-step-content">
                <p>${html(msg.init.modePrompt)}</p>
            </div>
            <div class="link-controls" id="init-controls">
                <button id="init-mode-local-btn" class="control-btn">${html(msg.init.modeLocal)}</button>
                <button id="init-mode-remote-btn" class="control-btn">${html(msg.init.modeRemote)}</button>
                <button id="init-mode-cancel-btn" class="control-btn">${html(msg.common.cancel)}</button>
            </div>
        </div>
    `;
    document.getElementById('init-mode-local-btn')?.addEventListener('click', () => displayInitLocalForm());
    document.getElementById('init-mode-remote-btn')?.addEventListener('click', () => displayInitRemoteForm());
    document.getElementById('init-mode-cancel-btn')?.addEventListener('click', () => displayHomeScreen());
}

function displayInitLocalForm() {
    const contentEl = document.getElementById('init-step-content');
    const controlsEl = document.getElementById('init-controls');
    if (contentEl) contentEl.textContent = msg.init.loadingDefaults;
    if (controlsEl) controlsEl.innerHTML = '';

    DefaultRepoPath().then((defaultPath) => {
        if (contentEl) contentEl.innerHTML = renderLocalForm(defaultPath);
        if (controlsEl) {
            controlsEl.innerHTML = `
                <button id="init-continue-btn" class="control-btn">${html(msg.common.continue)}</button>
                <button id="init-cancel-btn" class="control-btn">${html(msg.common.cancel)}</button>
            `;
        }
        document.getElementById('init-repo-browse-btn')?.addEventListener('click', () => {
            PickDirectory().then((path) => {
                if (!path) return;
                const input = document.getElementById('init-repo-path') as HTMLInputElement | null;
                if (input) input.value = path;
            }).catch((err) => {
                console.error(fmt(msg.init.pickDirectoryError, {error: String(err)}));
            });
        });
        document.getElementById('init-push-browse-btn')?.addEventListener('click', () => {
            PickDirectory().then((path) => {
                if (!path) return;
                const input = document.getElementById('init-push-target') as HTMLInputElement | null;
                if (input) input.value = path;
            }).catch((err) => {
                console.error(fmt(msg.init.pickDirectoryError, {error: String(err)}));
            });
        });
        document.getElementById('init-continue-btn')?.addEventListener('click', () => {
            const repoPath = (document.getElementById('init-repo-path') as HTMLInputElement | null)?.value ?? '';
            const pushTarget = (document.getElementById('init-push-target') as HTMLInputElement | null)?.value ?? '';
            runInitStart(() => StartInitLocal(repoPath, pushTarget));
        });
        document.getElementById('init-cancel-btn')?.addEventListener('click', () => displayHomeScreen());
    }).catch((err) => {
        showInitError(fmt(msg.init.defaultsError, {error: String(err)}));
    });
}

function displayInitRemoteForm() {
    const contentEl = document.getElementById('init-step-content');
    const controlsEl = document.getElementById('init-controls');
    if (contentEl) contentEl.textContent = msg.init.loadingDefaults;
    if (controlsEl) controlsEl.innerHTML = '';

    DefaultRepoPath().then((defaultPath) => {
        if (contentEl) contentEl.innerHTML = renderRemoteForm(defaultPath);
        if (controlsEl) {
            controlsEl.innerHTML = `
                <button id="init-continue-btn" class="control-btn">${html(msg.common.continue)}</button>
                <button id="init-cancel-btn" class="control-btn">${html(msg.common.cancel)}</button>
            `;
        }
        document.getElementById('init-clone-browse-btn')?.addEventListener('click', () => {
            PickDirectory().then((path) => {
                if (!path) return;
                const input = document.getElementById('init-clone-dir') as HTMLInputElement | null;
                if (input) input.value = path;
            }).catch((err) => {
                console.error(fmt(msg.init.pickDirectoryError, {error: String(err)}));
            });
        });
        document.getElementById('init-continue-btn')?.addEventListener('click', () => {
            const gitURL = (document.getElementById('init-git-url') as HTMLInputElement | null)?.value ?? '';
            const cloneDir = (document.getElementById('init-clone-dir') as HTMLInputElement | null)?.value ?? '';
            runInitStart(() => StartInitRemote(gitURL, cloneDir));
        });
        document.getElementById('init-cancel-btn')?.addEventListener('click', () => displayHomeScreen());
    }).catch((err) => {
        showInitError(fmt(msg.init.defaultsError, {error: String(err)}));
    });
}

function runInitStart(start: () => Promise<cli.InitStartInfo>) {
    const contentEl = document.getElementById('init-step-content');
    const controlsEl = document.getElementById('init-controls');
    if (contentEl) contentEl.textContent = msg.init.settingUp;
    if (controlsEl) controlsEl.innerHTML = '';

    start().then((info) => {
        if (info.collision) {
            displayInitCollision(info.collision.branch);
        } else {
            runFinishInit();
        }
    }).catch((err) => {
        showInitError(fmt(msg.init.setupError, {error: String(err)}));
    });
}

function displayInitCollision(branch: string) {
    const contentEl = document.getElementById('init-step-content');
    const controlsEl = document.getElementById('init-controls');
    if (contentEl) contentEl.innerHTML = renderBranchCollision(branch);
    if (controlsEl) {
        controlsEl.innerHTML = `
            <button id="init-collision-reuse-btn" class="control-btn">${html(msg.init.reuse)}</button>
            <button id="init-collision-unique-btn" class="control-btn">${html(msg.init.unique)}</button>
        `;
    }
    document.getElementById('init-collision-reuse-btn')?.addEventListener('click', () => resolveInitCollision(false));
    document.getElementById('init-collision-unique-btn')?.addEventListener('click', () => resolveInitCollision(true));
}

function resolveInitCollision(useUnique: boolean) {
    const contentEl = document.getElementById('init-step-content');
    const controlsEl = document.getElementById('init-controls');
    if (contentEl) contentEl.textContent = msg.init.settingUp;
    if (controlsEl) controlsEl.innerHTML = '';

    ResolveBranchCollision(useUnique).then(() => {
        runFinishInit();
    }).catch((err) => {
        showInitError(fmt(msg.init.collisionError, {error: String(err)}));
    });
}

function runFinishInit() {
    const contentEl = document.getElementById('init-step-content');
    const controlsEl = document.getElementById('init-controls');
    if (contentEl) contentEl.textContent = msg.init.finishing;
    if (controlsEl) controlsEl.innerHTML = '';

    FinishInit().then((result) => {
        if (contentEl) contentEl.innerHTML = renderInitResult(result);
        if (controlsEl) controlsEl.innerHTML = '<button id="init-done-continue-btn" class="control-btn">${html(msg.common.continue)}</button>';
        document.getElementById('init-done-continue-btn')?.addEventListener('click', () => displayHomeScreen());
    }).catch((err) => {
        showInitError(fmt(msg.init.finishError, {error: String(err)}));
    });
}

function showInitError(message: string) {
    const contentEl = document.getElementById('init-step-content');
    const controlsEl = document.getElementById('init-controls');
    if (contentEl) contentEl.textContent = message;
    if (controlsEl) controlsEl.innerHTML = '<button id="init-error-back-btn" class="control-btn">${html(msg.common.back)}</button>';
    document.getElementById('init-error-back-btn')?.addEventListener('click', () => displayHomeScreen());
}

function displayReportIssueView() {
    const app = document.querySelector('#app');
    if (!app) return;
    app.innerHTML = `
        <div class="link-container">
            <div class="link-header-section">
                <h1>${html(msg.reportIssue.title)}</h1>
            </div>
            <div id="report-issue-step-content">
                <p>${html(msg.reportIssue.warning)}</p>
                <label class="init-field-label" for="report-issue-text">${html(msg.reportIssue.prompt)}</label>
                <textarea id="report-issue-text" class="init-text-input report-issue-textarea"></textarea>
            </div>
            <div class="link-controls" id="report-issue-controls">
                <button id="report-issue-submit-btn" class="control-btn">${html(msg.reportIssue.submit)}</button>
                <button id="report-issue-cancel-btn" class="control-btn">${html(msg.common.cancel)}</button>
            </div>
        </div>
    `;
    document.getElementById('report-issue-submit-btn')?.addEventListener('click', () => {
        const text = (document.getElementById('report-issue-text') as HTMLTextAreaElement | null)?.value ?? '';
        runSubmitReportIssue(text);
    });
    document.getElementById('report-issue-cancel-btn')?.addEventListener('click', () => displayHomeScreen());
}

function runSubmitReportIssue(text: string) {
    const contentEl = document.getElementById('report-issue-step-content');
    const controlsEl = document.getElementById('report-issue-controls');
    if (contentEl) contentEl.textContent = msg.reportIssue.building;
    if (controlsEl) controlsEl.innerHTML = '';

    SubmitReportIssue(text).then((result) => {
        if (contentEl) contentEl.innerHTML = renderReportIssueResult(result);
        if (controlsEl) controlsEl.innerHTML = '<button id="report-issue-done-back-btn" class="control-btn">${html(msg.common.back)}</button>';
        document.getElementById('report-issue-done-back-btn')?.addEventListener('click', () => displayHomeScreen());
    }).catch((err) => {
        if (contentEl) contentEl.textContent = fmt(msg.reportIssue.error, {error: String(err)});
        if (controlsEl) controlsEl.innerHTML = '<button id="report-issue-error-back-btn" class="control-btn">${html(msg.common.back)}</button>';
        document.getElementById('report-issue-error-back-btn')?.addEventListener('click', () => displayHomeScreen());
    });
}

function displayPromoteView() {
    const app = document.querySelector('#app');
    if (!app) return;
    app.innerHTML = `
        <div class="link-container">
            <div class="link-header-section">
                <h1>${html(msg.promote.title)}</h1>
            </div>
            <div id="promote-step-content">${html(msg.promote.checking)}</div>
            <div class="link-controls" id="promote-controls"></div>
        </div>
    `;

    StartPromote().then((info) => {
        if (info.preserved.length > 0) {
            showPromotePreserved(info);
        } else if (info.diverged.length > 0) {
            runPromoteReview(info.diverged, 0);
        } else {
            runFinishPromote();
        }
    }).catch((err) => {
        showPromoteError(fmt(msg.promote.startError, {error: String(err)}));
    });
}

function showPromotePreserved(info: cli.PromoteStartInfo) {
    const contentEl = document.getElementById('promote-step-content');
    const controlsEl = document.getElementById('promote-controls');
    if (contentEl) contentEl.innerHTML = renderPreservedFiles(info.preserved);
    if (controlsEl) {
        controlsEl.innerHTML = `
            <button id="promote-preserved-continue-btn" class="control-btn">${html(msg.common.continue)}</button>
            <button id="promote-preserved-cancel-btn" class="control-btn">${html(msg.common.cancel)}</button>
        `;
    }
    document.getElementById('promote-preserved-continue-btn')?.addEventListener('click', () => {
        if (info.diverged.length > 0) {
            runPromoteReview(info.diverged, 0);
        } else {
            runFinishPromote();
        }
    });
    document.getElementById('promote-preserved-cancel-btn')?.addEventListener('click', () => displayHomeScreen());
}

function runPromoteReview(files: cli.DivergedFile[], index: number) {
    const contentEl = document.getElementById('promote-step-content');
    const controlsEl = document.getElementById('promote-controls');
    if (contentEl) contentEl.innerHTML = renderDivergedFileReview(files[index], index, files.length);
    if (controlsEl) {
        controlsEl.innerHTML = `
            <button id="promote-keep-mine-btn" class="control-btn">${html(msg.promote.keepMine)}</button>
            <button id="promote-keep-theirs-btn" class="control-btn">${html(msg.promote.keepTheirs)}</button>
        `;
    }
    const advance = () => {
        const next = index + 1;
        if (next < files.length) {
            runPromoteReview(files, next);
        } else {
            runFinishPromote();
        }
    };
    document.getElementById('promote-keep-mine-btn')?.addEventListener('click', () => {
        if (controlsEl) controlsEl.innerHTML = '';
        ResolveDivergedFile(index, true).then(advance).catch((err) => {
            showPromoteError(fmt(msg.promote.resolveError, {path: files[index].path, error: String(err)}));
        });
    });
    document.getElementById('promote-keep-theirs-btn')?.addEventListener('click', () => {
        if (controlsEl) controlsEl.innerHTML = '';
        ResolveDivergedFile(index, false).then(advance).catch((err) => {
            showPromoteError(fmt(msg.promote.resolveError, {path: files[index].path, error: String(err)}));
        });
    });
}

function runFinishPromote() {
    const contentEl = document.getElementById('promote-step-content');
    const controlsEl = document.getElementById('promote-controls');
    if (contentEl) contentEl.textContent = msg.promote.promoting;
    if (controlsEl) controlsEl.innerHTML = '';

    FinishPromote().then((result) => {
        if (contentEl) contentEl.innerHTML = renderPromoteResult(result);
        if (controlsEl) controlsEl.innerHTML = '<button id="promote-done-continue-btn" class="control-btn">${html(msg.common.continue)}</button>';
        document.getElementById('promote-done-continue-btn')?.addEventListener('click', () => displayHomeScreen());
    }).catch((err) => {
        showPromoteError(fmt(msg.promote.error, {error: String(err)}));
    });
}

function showPromoteError(message: string) {
    const contentEl = document.getElementById('promote-step-content');
    const controlsEl = document.getElementById('promote-controls');
    if (contentEl) contentEl.textContent = message;
    if (controlsEl) controlsEl.innerHTML = '<button id="promote-error-back-btn" class="control-btn">${html(msg.common.back)}</button>';
    document.getElementById('promote-error-back-btn')?.addEventListener('click', () => displayHomeScreen());
}

function displayLinkView(noFetch: boolean) {
    const app = document.querySelector('#app');
    if (!app) return;
    app.innerHTML = `
        <div class="link-container">
            <div class="link-header-section">
                <h1>${html(msg.link.title)}</h1>
            </div>
            <div id="link-step-content">${html(msg.warnings.checking)}</div>
            <div class="link-controls" id="link-controls"></div>
        </div>
    `;
    runLinkWarningsStep(noFetch);
}

function runLinkWarningsStep(noFetch: boolean) {
    GetPendingWarnings().then((warnings) => {
        if (warnings.length > 0) {
            showLinkWarnings(warnings, noFetch);
        } else {
            runLinkStartStep(noFetch);
        }
    }).catch((err) => {
        showLinkError(fmt(msg.warnings.checkError, {error: String(err)}));
    });
}

function showLinkWarnings(warnings: string[], noFetch: boolean) {
    const contentEl = document.getElementById('link-step-content');
    const controlsEl = document.getElementById('link-controls');
    if (contentEl) contentEl.innerHTML = renderPendingWarnings(warnings);
    if (controlsEl) {
        controlsEl.innerHTML = `
            <button id="link-warnings-continue-btn" class="control-btn">${html(msg.common.continue)}</button>
            <button id="link-warnings-cancel-btn" class="control-btn">${html(msg.common.cancel)}</button>
        `;
    }
    document.getElementById('link-warnings-continue-btn')?.addEventListener('click', () => runLinkStartStep(noFetch));
    document.getElementById('link-warnings-cancel-btn')?.addEventListener('click', () => displayHomeScreen());
}

function runLinkStartStep(noFetch: boolean) {
    const contentEl = document.getElementById('link-step-content');
    const controlsEl = document.getElementById('link-controls');
    if (contentEl) contentEl.textContent = noFetch ? msg.link.skippingFetch : msg.link.fetching;
    if (controlsEl) controlsEl.innerHTML = '';

    StartLink(noFetch).then((info) => {
        if (info.incomingFiles.length > 0) {
            runLinkReviewStep(info.incomingFiles, 0);
        } else {
            runLinkFinishStep(info.message);
        }
    }).catch((err) => {
        showLinkError(fmt(msg.link.startError, {error: String(err)}));
    });
}

function runLinkReviewStep(files: cli.IncomingFile[], index: number) {
    const contentEl = document.getElementById('link-step-content');
    const controlsEl = document.getElementById('link-controls');
    if (contentEl) contentEl.innerHTML = renderIncomingFileReview(files[index], index, files.length);
    if (controlsEl) {
        controlsEl.innerHTML = `
            <button id="link-accept-btn" class="control-btn">${html(msg.link.accept)}</button>
            <button id="link-skip-btn" class="control-btn">${html(msg.link.skip)}</button>
        `;
    }
    document.getElementById('link-accept-btn')?.addEventListener('click', () => {
        if (controlsEl) controlsEl.innerHTML = '';
        AcceptIncomingFile(index).then(() => {
            advanceLinkReview(files, index);
        }).catch((err) => {
            showLinkAcceptError(files, index, String(err));
        });
    });
    document.getElementById('link-skip-btn')?.addEventListener('click', () => {
        advanceLinkReview(files, index);
    });
}

function advanceLinkReview(files: cli.IncomingFile[], index: number) {
    const next = index + 1;
    if (next < files.length) {
        runLinkReviewStep(files, next);
    } else {
        runLinkFinishStep('');
    }
}

function showLinkAcceptError(files: cli.IncomingFile[], index: number, errMessage: string) {
    const contentEl = document.getElementById('link-step-content');
    const controlsEl = document.getElementById('link-controls');
    if (contentEl) {
        contentEl.innerHTML = '<p class="link-error"></p>';
        const errorEl = contentEl.querySelector('.link-error');
        if (errorEl) errorEl.textContent = fmt(msg.link.acceptError, {path: files[index].path, error: errMessage});
    }
    if (controlsEl) {
        controlsEl.innerHTML = `
            <button id="link-error-skip-btn" class="control-btn">${html(msg.link.skipAndContinue)}</button>
            <button id="link-error-cancel-btn" class="control-btn">${html(msg.common.cancel)}</button>
        `;
    }
    document.getElementById('link-error-skip-btn')?.addEventListener('click', () => advanceLinkReview(files, index));
    document.getElementById('link-error-cancel-btn')?.addEventListener('click', () => displayHomeScreen());
}

function runLinkFinishStep(message: string) {
    const contentEl = document.getElementById('link-step-content');
    const controlsEl = document.getElementById('link-controls');
    if (contentEl) contentEl.textContent = msg.link.relinking;
    if (controlsEl) controlsEl.innerHTML = '';

    FinishLink().then((results) => {
        if (contentEl) contentEl.innerHTML = renderLinkResults(message, results);
        if (controlsEl) controlsEl.innerHTML = '<button id="link-done-back-btn" class="control-btn">${html(msg.common.back)}</button>';
        document.getElementById('link-done-back-btn')?.addEventListener('click', () => displayHomeScreen());
    }).catch((err) => {
        showLinkError(fmt(msg.link.finishError, {error: String(err)}));
    });
}

function showLinkError(message: string) {
    const contentEl = document.getElementById('link-step-content');
    const controlsEl = document.getElementById('link-controls');
    if (contentEl) contentEl.textContent = message;
    if (controlsEl) controlsEl.innerHTML = '<button id="link-error-back-btn" class="control-btn">${html(msg.common.back)}</button>';
    document.getElementById('link-error-back-btn')?.addEventListener('click', () => displayHomeScreen());
}

function loadCurrentDiff() {
    const loadingEl = document.getElementById('loading');
    const diffEl = document.getElementById('diff-content');

    if (loadingEl) loadingEl.style.display = 'block';
    if (diffEl) diffEl.style.display = 'none';

    GetDiffContent().then((content) => {
        if (loadingEl) loadingEl.style.display = 'none';
        if (diffEl) {
            diffEl.innerHTML = renderDiffContent(content);
            diffEl.style.display = 'block';
        }
        updateNavigationState();
    }).catch((err) => {
        if (loadingEl) loadingEl.textContent = fmt(msg.diff.loadError, {error: String(err)});
        updateNavigationState();
    });
}

function updateNavigationState() {
    Promise.all([GetCurrentIndex(), GetTotalDiffs()]).then(([currentIndex, totalDiffs]) => {
        const counterEl = document.getElementById('diff-counter');
        const prevBtn = document.getElementById('prev-btn') as HTMLButtonElement;
        const nextBtn = document.getElementById('next-btn') as HTMLButtonElement;
        if (counterEl) counterEl.textContent = fmt(msg.diff.counter, {index: currentIndex + 1, total: totalDiffs});
        if (prevBtn) prevBtn.disabled = currentIndex === 0;
        if (nextBtn) nextBtn.disabled = currentIndex === totalDiffs - 1;
    });
}

function displayDiffViewer() {
    document.querySelector('#app')!.innerHTML = `
        <div class="diff-container">
            <div class="diff-header-section">
                <h1>${html(msg.diff.title)}</h1>
                <div id="diff-counter" class="diff-counter"></div>
            </div>
            <div id="loading">${html(msg.diff.loading)}</div>
            <div id="diff-content" style="display: none;"></div>
            <div class="diff-controls">
                <button id="prev-btn" class="control-btn">${html(msg.diff.previous)}</button>
                <button id="next-btn" class="control-btn">${html(msg.diff.next)}</button>
                <button id="close-btn" class="control-btn close-btn">${html(msg.common.close)}</button>
            </div>
        </div>
    `;

    const prevBtn = document.getElementById('prev-btn') as HTMLButtonElement | null;
    const nextBtn = document.getElementById('next-btn') as HTMLButtonElement | null;

    prevBtn?.addEventListener('click', () => {
        if (prevBtn) prevBtn.disabled = true;
        if (nextBtn) nextBtn.disabled = true;
        PreviousDiff().then(loadCurrentDiff);
    });
    nextBtn?.addEventListener('click', () => {
        if (prevBtn) prevBtn.disabled = true;
        if (nextBtn) nextBtn.disabled = true;
        NextDiff().then(loadCurrentDiff);
    });
    document.getElementById('close-btn')?.addEventListener('click', () => CloseWindow());

    loadCurrentDiff();
}
