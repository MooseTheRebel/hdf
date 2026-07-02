export function renderDiffContent(content: string): string {
    const lines = content.endsWith('\n') ? content.slice(0, -1).split('\n') : content.split('\n');
    const htmlLines = lines.map(line => {
        let className = 'diff-line';
        if (line.startsWith('diff ') || line.startsWith('index ') || line.startsWith('---') || line.startsWith('+++')) {
            className += ' diff-header';
        } else if (line.startsWith('@@')) {
            className += ' diff-hunk';
        } else if (line.startsWith('+')) {
            className += ' diff-addition';
        } else if (line.startsWith('-')) {
            className += ' diff-deletion';
        }
        const escapedLine = line
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;');
        return `<div class="${className}">${escapedLine || ' '}</div>`;
    });
    return htmlLines.join('');
}
