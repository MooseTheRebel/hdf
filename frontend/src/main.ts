import './style.css';
import './app.css';

import {IsInitialized, HasDiff, GetDiffContent, GetCurrentIndex, GetTotalDiffs, NextDiff, PreviousDiff, CloseWindow, GetStatus, GetConfig, GetDaemonStatus, InstallDaemon, UninstallDaemon, StartDaemon, StopDaemon, GetPendingWarnings, StartLink, AcceptIncomingFile, FinishLink, PickFileToEnroll, StartEnroll, ConfirmEnroll} from '../wailsjs/go/cli/App';
import type {cli} from '../wailsjs/go/models';
import {renderDiffContent} from './diff';
import {renderStatus} from './status';
import {renderConfig} from './configinfo';
import {renderDaemonStatus} from './daemonstatus';
import {renderDaemonManagement} from './daemonmanagement';
import {renderPendingWarnings, renderIncomingFileReview, renderLinkResults} from './linkflow';
import {renderEnrollPreview, renderEnrollResult} from './enrollflow';

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
            app.innerHTML = `
                <div class="home-container">
                    <div class="home-header">
                        <h1 class="home-title">home-dawt-files</h1>
                        <span class="home-badge initialized">initialized</span>
                    </div>
                    <p class="home-subtitle">Your dotfiles are managed by hdf.</p>
                    <div class="command-list">
                        <button class="command-row command-row-clickable" id="enroll-btn">
                            <code class="cmd">hdf enroll &lt;path&gt;</code>
                            <span class="cmd-desc">Start managing a new dotfile</span>
                        </button>
                        <button class="command-row command-row-clickable" id="link-btn">
                            <code class="cmd">hdf link</code>
                            <span class="cmd-desc">Re-create all managed symlinks</span>
                        </button>
                        <button class="command-row command-row-clickable" id="link-no-fetch-btn">
                            <code class="cmd">hdf link --no-fetch</code>
                            <span class="cmd-desc">Re-create symlinks without fetching from remote</span>
                        </button>
                        <button class="command-row command-row-clickable" id="status-btn">
                            <code class="cmd">hdf status</code>
                            <span class="cmd-desc">Show managed files and sync state</span>
                        </button>
                        <div class="command-row">
                            <code class="cmd">hdf daemon</code>
                            <span class="cmd-desc">Start the background sync daemon</span>
                        </div>
                        <div class="command-row">
                            <code class="cmd">hdf diff [url]</code>
                            <span class="cmd-desc">View a diff in this window</span>
                        </div>
                        <button class="command-row command-row-clickable" id="config-btn">
                            <code class="cmd">hdf config</code>
                            <span class="cmd-desc">Show the current configuration</span>
                        </button>
                        <button class="command-row command-row-clickable" id="daemon-status-btn">
                            <code class="cmd">hdf daemon status</code>
                            <span class="cmd-desc">Check whether the sync daemon service is running</span>
                        </button>
                        <button class="command-row command-row-clickable" id="daemon-management-btn">
                            <code class="cmd">hdf daemon install/start/stop/uninstall</code>
                            <span class="cmd-desc">Manage the sync daemon background service</span>
                        </button>
                    </div>
                    <button class="close-button" id="close-btn">Close</button>
                </div>
            `;
        } else {
            app.innerHTML = `
                <div class="home-container">
                    <div class="home-header">
                        <h1 class="home-title">home-dawt-files</h1>
                        <span class="home-badge not-initialized">not initialized</span>
                    </div>
                    <p class="home-subtitle">Manage your dotfiles with git — across every machine.</p>
                    <div class="steps">
                        <div class="step">
                            <span class="step-number">1</span>
                            <div class="step-body">
                                <div class="step-label">Initialize hdf</div>
                                <code class="step-cmd">hdf init</code>
                                <div class="step-hint">Sets up a local git repo and push target.</div>
                            </div>
                        </div>
                        <div class="step">
                            <span class="step-number">2</span>
                            <div class="step-body">
                                <div class="step-label">Enroll a dotfile</div>
                                <code class="step-cmd">hdf enroll ~/.bashrc</code>
                                <div class="step-hint">Copies the file into the repo and replaces it with a symlink.</div>
                            </div>
                        </div>
                        <div class="step">
                            <span class="step-number">3</span>
                            <div class="step-body">
                                <div class="step-label">On a new machine — re-link</div>
                                <code class="step-cmd">hdf link</code>
                                <div class="step-hint">Recreates symlinks for all managed files after cloning.</div>
                            </div>
                        </div>
                        <div class="step">
                            <span class="step-number">4</span>
                            <div class="step-body">
                                <div class="step-label">Check drift</div>
                                <code class="step-cmd">hdf status</code>
                                <div class="step-hint">Shows which files have uncommitted local changes.</div>
                            </div>
                        </div>
                    </div>
                    <button class="close-button" id="close-btn">Close</button>
                </div>
            `;
        }

        document.getElementById('close-btn')?.addEventListener('click', () => CloseWindow());
        document.getElementById('enroll-btn')?.addEventListener('click', () => startEnrollFlow());
        document.getElementById('link-btn')?.addEventListener('click', () => displayLinkView(false));
        document.getElementById('link-no-fetch-btn')?.addEventListener('click', () => displayLinkView(true));
        document.getElementById('status-btn')?.addEventListener('click', () => displayStatusView());
        document.getElementById('config-btn')?.addEventListener('click', () => displayConfigView());
        document.getElementById('daemon-status-btn')?.addEventListener('click', () => displayDaemonStatusView());
        document.getElementById('daemon-management-btn')?.addEventListener('click', () => displayDaemonManagementView());
    }).catch((err) => {
        app.innerHTML = `
            <div class="home-container">
                <div class="home-header">
                    <h1 class="home-title">home-dawt-files</h1>
                    <span class="home-badge not-initialized">error</span>
                </div>
                <p class="home-subtitle">Could not read hdf configuration.</p>
                <p class="home-subtitle" id="error-message"></p>
                <button class="close-button" id="error-close-btn">Close</button>
            </div>
        `;
        const errorMsgEl = document.getElementById('error-message');
        if (errorMsgEl) errorMsgEl.textContent = String(err);
        document.getElementById('error-close-btn')?.addEventListener('click', () => CloseWindow());
    });
}

function displayStatusView() {
    const app = document.querySelector('#app');
    if (!app) return;
    app.innerHTML = `
        <div class="status-container">
            <div class="status-header-section">
                <h1>Status</h1>
            </div>
            <div id="status-loading">Loading status...</div>
            <div id="status-content" style="display: none;"></div>
            <div class="status-controls">
                <button id="status-back-btn" class="control-btn">Back</button>
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
        if (loadingEl) loadingEl.textContent = 'Error loading status: ' + err;
    });
}

function displayConfigView() {
    const app = document.querySelector('#app');
    if (!app) return;
    app.innerHTML = `
        <div class="config-container">
            <div class="config-header-section">
                <h1>Config</h1>
            </div>
            <div id="config-loading">Loading config...</div>
            <div id="config-content" style="display: none;"></div>
            <div class="config-controls">
                <button id="config-back-btn" class="control-btn">Back</button>
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
        if (loadingEl) loadingEl.textContent = 'Error loading config: ' + err;
    });
}

function displayDaemonStatusView() {
    const app = document.querySelector('#app');
    if (!app) return;
    app.innerHTML = `
        <div class="daemon-status-container">
            <div class="daemon-status-header-section">
                <h1>Daemon Status</h1>
            </div>
            <div id="daemon-status-loading">Loading daemon status...</div>
            <div id="daemon-status-content" style="display: none;"></div>
            <div class="daemon-status-controls">
                <button id="daemon-status-back-btn" class="control-btn">Back</button>
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
        if (loadingEl) loadingEl.textContent = 'Error loading daemon status: ' + err;
    });
}

function displayDaemonManagementView() {
    const app = document.querySelector('#app');
    if (!app) return;
    app.innerHTML = `
        <div class="daemon-management-container">
            <div class="daemon-management-header-section">
                <h1>Daemon Management</h1>
            </div>
            <div id="daemon-management-loading">Loading daemon status...</div>
            <div id="daemon-management-content" style="display: none;"></div>
            <div id="daemon-management-result"></div>
            <div class="daemon-management-controls">
                <button id="daemon-management-back-btn" class="control-btn">Back</button>
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
        if (loadingEl) loadingEl.textContent = 'Error loading daemon status: ' + err;
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
            if (resultEl) resultEl.textContent = 'Error: ' + err;
        });
    };

    document.getElementById('daemon-install-btn')?.addEventListener('click', () => runAction(InstallDaemon, 'Daemon installed and started.'));
    document.getElementById('daemon-uninstall-btn')?.addEventListener('click', () => runAction(UninstallDaemon, 'Daemon uninstalled.'));
    document.getElementById('daemon-start-btn')?.addEventListener('click', () => runAction(StartDaemon, 'Daemon started.'));
    document.getElementById('daemon-stop-btn')?.addEventListener('click', () => runAction(StopDaemon, 'Daemon stopped.'));
}

function startEnrollFlow() {
    PickFileToEnroll().then((path) => {
        if (!path) return;
        displayEnrollView(path);
    }).catch((err) => {
        displayEnrollErrorScreen('Error picking a file: ' + err);
    });
}

function displayEnrollView(path: string) {
    const app = document.querySelector('#app');
    if (!app) return;
    app.innerHTML = `
        <div class="link-container">
            <div class="link-header-section">
                <h1>Enroll</h1>
            </div>
            <div id="enroll-step-content">Checking for pending warnings...</div>
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
                <h1>Enroll</h1>
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
        showEnrollError('Error checking pending warnings: ' + err);
    });
}

function showEnrollWarnings(warnings: string[], path: string) {
    const contentEl = document.getElementById('enroll-step-content');
    const controlsEl = document.getElementById('enroll-controls');
    if (contentEl) contentEl.innerHTML = renderPendingWarnings(warnings);
    if (controlsEl) {
        controlsEl.innerHTML = `
            <button id="enroll-warnings-continue-btn" class="control-btn">Continue</button>
            <button id="enroll-warnings-cancel-btn" class="control-btn">Cancel</button>
        `;
    }
    document.getElementById('enroll-warnings-continue-btn')?.addEventListener('click', () => runEnrollStartStep(path));
    document.getElementById('enroll-warnings-cancel-btn')?.addEventListener('click', () => displayHomeScreen());
}

function runEnrollStartStep(path: string) {
    const contentEl = document.getElementById('enroll-step-content');
    const controlsEl = document.getElementById('enroll-controls');
    if (contentEl) contentEl.textContent = 'Preparing enroll preview...';
    if (controlsEl) controlsEl.innerHTML = '';

    StartEnroll(path).then((info) => {
        if (contentEl) contentEl.innerHTML = renderEnrollPreview(info);
        if (controlsEl) {
            controlsEl.innerHTML = `
                <button id="enroll-confirm-btn" class="control-btn">Enroll</button>
                <button id="enroll-cancel-btn" class="control-btn">Cancel</button>
            `;
        }
        document.getElementById('enroll-confirm-btn')?.addEventListener('click', () => runEnrollConfirmStep());
        document.getElementById('enroll-cancel-btn')?.addEventListener('click', () => displayHomeScreen());
    }).catch((err) => {
        showEnrollError('Error starting enroll: ' + err);
    });
}

function runEnrollConfirmStep() {
    const contentEl = document.getElementById('enroll-step-content');
    const controlsEl = document.getElementById('enroll-controls');
    if (contentEl) contentEl.textContent = 'Enrolling...';
    if (controlsEl) controlsEl.innerHTML = '';

    ConfirmEnroll().then((result) => {
        if (contentEl) contentEl.innerHTML = renderEnrollResult(result);
        if (controlsEl) controlsEl.innerHTML = '<button id="enroll-done-back-btn" class="control-btn">Back</button>';
        document.getElementById('enroll-done-back-btn')?.addEventListener('click', () => displayHomeScreen());
    }).catch((err) => {
        showEnrollError('Error enrolling: ' + err);
    });
}

function showEnrollError(message: string) {
    const contentEl = document.getElementById('enroll-step-content');
    const controlsEl = document.getElementById('enroll-controls');
    if (contentEl) contentEl.textContent = message;
    if (controlsEl) controlsEl.innerHTML = '<button id="enroll-error-back-btn" class="control-btn">Back</button>';
    document.getElementById('enroll-error-back-btn')?.addEventListener('click', () => displayHomeScreen());
}

function displayLinkView(noFetch: boolean) {
    const app = document.querySelector('#app');
    if (!app) return;
    app.innerHTML = `
        <div class="link-container">
            <div class="link-header-section">
                <h1>Link</h1>
            </div>
            <div id="link-step-content">Checking for pending warnings...</div>
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
        showLinkError('Error checking pending warnings: ' + err);
    });
}

function showLinkWarnings(warnings: string[], noFetch: boolean) {
    const contentEl = document.getElementById('link-step-content');
    const controlsEl = document.getElementById('link-controls');
    if (contentEl) contentEl.innerHTML = renderPendingWarnings(warnings);
    if (controlsEl) {
        controlsEl.innerHTML = `
            <button id="link-warnings-continue-btn" class="control-btn">Continue</button>
            <button id="link-warnings-cancel-btn" class="control-btn">Cancel</button>
        `;
    }
    document.getElementById('link-warnings-continue-btn')?.addEventListener('click', () => runLinkStartStep(noFetch));
    document.getElementById('link-warnings-cancel-btn')?.addEventListener('click', () => displayHomeScreen());
}

function runLinkStartStep(noFetch: boolean) {
    const contentEl = document.getElementById('link-step-content');
    const controlsEl = document.getElementById('link-controls');
    if (contentEl) contentEl.textContent = noFetch ? 'Skipping fetch...' : 'Fetching from remote...';
    if (controlsEl) controlsEl.innerHTML = '';

    StartLink(noFetch).then((info) => {
        if (info.incomingFiles.length > 0) {
            runLinkReviewStep(info.incomingFiles, 0);
        } else {
            runLinkFinishStep(info.message);
        }
    }).catch((err) => {
        showLinkError('Error starting link: ' + err);
    });
}

function runLinkReviewStep(files: cli.IncomingFile[], index: number) {
    const contentEl = document.getElementById('link-step-content');
    const controlsEl = document.getElementById('link-controls');
    if (contentEl) contentEl.innerHTML = renderIncomingFileReview(files[index], index, files.length);
    if (controlsEl) {
        controlsEl.innerHTML = `
            <button id="link-accept-btn" class="control-btn">Accept</button>
            <button id="link-skip-btn" class="control-btn">Skip</button>
        `;
    }
    document.getElementById('link-accept-btn')?.addEventListener('click', () => {
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
        if (errorEl) errorEl.textContent = `Error accepting ${files[index].path}: ${errMessage}`;
    }
    if (controlsEl) {
        controlsEl.innerHTML = `
            <button id="link-error-skip-btn" class="control-btn">Skip and continue</button>
            <button id="link-error-cancel-btn" class="control-btn">Cancel</button>
        `;
    }
    document.getElementById('link-error-skip-btn')?.addEventListener('click', () => advanceLinkReview(files, index));
    document.getElementById('link-error-cancel-btn')?.addEventListener('click', () => displayHomeScreen());
}

function runLinkFinishStep(message: string) {
    const contentEl = document.getElementById('link-step-content');
    const controlsEl = document.getElementById('link-controls');
    if (contentEl) contentEl.textContent = 'Re-creating symlinks...';
    if (controlsEl) controlsEl.innerHTML = '';

    FinishLink().then((results) => {
        if (contentEl) contentEl.innerHTML = renderLinkResults(message, results);
        if (controlsEl) controlsEl.innerHTML = '<button id="link-done-back-btn" class="control-btn">Back</button>';
        document.getElementById('link-done-back-btn')?.addEventListener('click', () => displayHomeScreen());
    }).catch((err) => {
        showLinkError('Error finishing link: ' + err);
    });
}

function showLinkError(message: string) {
    const contentEl = document.getElementById('link-step-content');
    const controlsEl = document.getElementById('link-controls');
    if (contentEl) contentEl.textContent = message;
    if (controlsEl) controlsEl.innerHTML = '<button id="link-error-back-btn" class="control-btn">Back</button>';
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
        if (loadingEl) loadingEl.textContent = 'Error loading diff: ' + err;
        updateNavigationState();
    });
}

function updateNavigationState() {
    Promise.all([GetCurrentIndex(), GetTotalDiffs()]).then(([currentIndex, totalDiffs]) => {
        const counterEl = document.getElementById('diff-counter');
        const prevBtn = document.getElementById('prev-btn') as HTMLButtonElement;
        const nextBtn = document.getElementById('next-btn') as HTMLButtonElement;
        if (counterEl) counterEl.textContent = `Diff ${currentIndex + 1} of ${totalDiffs}`;
        if (prevBtn) prevBtn.disabled = currentIndex === 0;
        if (nextBtn) nextBtn.disabled = currentIndex === totalDiffs - 1;
    });
}

function displayDiffViewer() {
    document.querySelector('#app')!.innerHTML = `
        <div class="diff-container">
            <div class="diff-header-section">
                <h1>Diff Viewer</h1>
                <div id="diff-counter" class="diff-counter"></div>
            </div>
            <div id="loading">Loading diff...</div>
            <div id="diff-content" style="display: none;"></div>
            <div class="diff-controls">
                <button id="prev-btn" class="control-btn">Previous</button>
                <button id="next-btn" class="control-btn">Next</button>
                <button id="close-btn" class="control-btn close-btn">Close</button>
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
