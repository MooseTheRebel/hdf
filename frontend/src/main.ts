import './style.css';
import './app.css';

import {IsInitialized, HasDiff, GetDiffContent, GetCurrentIndex, GetTotalDiffs, NextDiff, PreviousDiff, CloseWindow} from '../wailsjs/go/main/App';
import {renderDiffContent} from './diff';

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
                        <div class="command-row">
                            <code class="cmd">hdf enroll &lt;path&gt;</code>
                            <span class="cmd-desc">Start managing a new dotfile</span>
                        </div>
                        <div class="command-row">
                            <code class="cmd">hdf link</code>
                            <span class="cmd-desc">Re-create all managed symlinks</span>
                        </div>
                        <div class="command-row">
                            <code class="cmd">hdf status</code>
                            <span class="cmd-desc">Show managed files and sync state</span>
                        </div>
                        <div class="command-row">
                            <code class="cmd">hdf daemon</code>
                            <span class="cmd-desc">Start the background sync daemon</span>
                        </div>
                        <div class="command-row">
                            <code class="cmd">hdf diff [url]</code>
                            <span class="cmd-desc">View a diff in this window</span>
                        </div>
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
