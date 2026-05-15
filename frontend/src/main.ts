import './style.css';
import './app.css';

import logo from './assets/images/logo-universal.png';
import {Greet, HasDiff, GetDiffContent, GetCurrentIndex, GetTotalDiffs, NextDiff, PreviousDiff, CloseWindow} from '../wailsjs/go/main/App';

// Check if we're in diff mode
HasDiff().then((hasDiff) => {
    if (hasDiff) {
        // Display diff viewer
        displayDiffViewer();
    } else {
        // Display normal greet interface
        displayGreetInterface();
    }
}).catch((err) => {
    console.error('Error checking diff mode:', err);
    displayGreetInterface();
});

function loadCurrentDiff() {
    const loadingEl = document.getElementById('loading');
    const diffEl = document.getElementById('diff-content');

    if (loadingEl) loadingEl.style.display = 'block';
    if (diffEl) diffEl.style.display = 'none';

    GetDiffContent().then((content) => {
        if (loadingEl) loadingEl.style.display = 'none';
        if (diffEl) {
            // Parse and render diff with highlighting
            const lines = content.split('\n');
            const htmlLines = lines.map(line => {
                let className = 'diff-line';
                if (line.startsWith('+')) {
                    className += ' diff-addition';
                } else if (line.startsWith('-')) {
                    className += ' diff-deletion';
                } else if (line.startsWith('@@')) {
                    className += ' diff-hunk';
                } else if (line.startsWith('diff ') || line.startsWith('index ') || line.startsWith('---') || line.startsWith('+++')) {
                    className += ' diff-header';
                }

                // Escape HTML to prevent injection
                const escapedLine = line
                    .replace(/&/g, '&amp;')
                    .replace(/</g, '&lt;')
                    .replace(/>/g, '&gt;');

                return `<div class="${className}">${escapedLine || ' '}</div>`;
            });

            diffEl.innerHTML = htmlLines.join('');
            diffEl.style.display = 'block';
        }

        // Update button states and counter
        updateNavigationState();
    }).catch((err) => {
        console.error('Error loading diff:', err);
        if (loadingEl) {
            loadingEl.textContent = 'Error loading diff: ' + err;
        }
    });
}

function updateNavigationState() {
    Promise.all([GetCurrentIndex(), GetTotalDiffs()]).then(([currentIndex, totalDiffs]) => {
        const counterEl = document.getElementById('diff-counter');
        const prevBtn = document.getElementById('prev-btn') as HTMLButtonElement;
        const nextBtn = document.getElementById('next-btn') as HTMLButtonElement;

        if (counterEl) {
            counterEl.textContent = `Diff ${currentIndex + 1} of ${totalDiffs}`;
        }

        if (prevBtn) {
            prevBtn.disabled = currentIndex === 0;
        }

        if (nextBtn) {
            nextBtn.disabled = currentIndex === totalDiffs - 1;
        }
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

    // Set up button handlers
    const prevBtn = document.getElementById('prev-btn');
    const nextBtn = document.getElementById('next-btn');
    const closeBtn = document.getElementById('close-btn');

    if (prevBtn) {
        prevBtn.addEventListener('click', () => {
            PreviousDiff().then(() => {
                loadCurrentDiff();
            });
        });
    }

    if (nextBtn) {
        nextBtn.addEventListener('click', () => {
            NextDiff().then(() => {
                loadCurrentDiff();
            });
        });
    }

    if (closeBtn) {
        closeBtn.addEventListener('click', () => {
            CloseWindow();
        });
    }

    // Load the initial diff
    loadCurrentDiff();
}

function displayGreetInterface() {
    // Setup the greet function
    (window as any).greet = function () {
        // Get name
        let name = nameElement!.value;

        // Check if the input is empty
        if (name === "") return;

        // Call App.Greet(name)
        try {
            Greet(name)
                .then((result) => {
                    // Update result with data back from App.Greet()
                    resultElement!.innerText = result;
                })
                .catch((err) => {
                    console.error(err);
                });
        } catch (err) {
            console.error(err);
        }
    };

    document.querySelector('#app')!.innerHTML = `
        <img id="logo" class="logo">
          <div class="result" id="result">Please enter your name below 👇</div>
          <div class="input-box" id="input">
            <input class="input" id="name" type="text" autocomplete="off" />
            <button class="btn" onclick="greet()">Greet</button>
          </div>
        </div>
    `;
    (document.getElementById('logo') as HTMLImageElement).src = logo;

    let nameElement = (document.getElementById("name") as HTMLInputElement);
    nameElement.focus();
    let resultElement = document.getElementById("result");
}
