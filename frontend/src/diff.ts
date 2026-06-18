export function renderDiffContent(content: string): string {
    const lines = content.split('\n');
    const htmlLines = lines.map(line => {
        let className = 'diff-line';
        if (line.startsWith('+')) {
            className += ' diff-addition';
        } else if (line.startsWith('-')) {
            className += ' diff-deletion';
        } else if (line.startsWith('@@')) {
            className += ' diff-hunk';
        } else if (line.startsWith('diff ') || line.startsWith('index ')) {
            className += ' diff-header';
        }
        const escapedLine = line
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;');
        return `<div class="${className}">${escapedLine || ' '}</div>`;
    });
    return htmlLines.join('');
}
