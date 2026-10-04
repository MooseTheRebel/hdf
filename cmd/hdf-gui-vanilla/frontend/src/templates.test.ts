import {describe, it, expect} from 'vitest';
import {readFileSync, readdirSync} from 'node:fs';
import {parseSync} from 'vite';

// `${...}` only interpolates inside backtick template literals; in a '...'
// or "..." string it renders literally (e.g. a button reading
// "${html(msg.common.back)}"). Neither tsc nor the render-function tests
// catch that in main.ts, which has no tests of its own, so parse every
// source file and flag plain string literals containing "${".
function stringLiteralsWithInterpolation(file: string, source: string): string[] {
    const {program, errors} = parseSync(file, source, {lang: 'ts'});
    expect(errors).toEqual([]);
    const found: string[] = [];
    const walk = (node: unknown): void => {
        if (Array.isArray(node)) {
            node.forEach(walk);
        } else if (node && typeof node === 'object') {
            const n = node as {type?: string; value?: unknown};
            if (n.type === 'Literal' && typeof n.value === 'string' && n.value.includes('${')) {
                found.push(n.value);
            }
            Object.values(node).forEach(walk);
        }
    };
    walk(program);
    return found;
}

describe('interpolation', () => {
    const dir = new URL('.', import.meta.url);
    const files = readdirSync(dir).filter((f) => f.endsWith('.ts') && !f.endsWith('.test.ts'));

    it.each(files)('%s only uses ${...} inside template literals', (file) => {
        expect(stringLiteralsWithInterpolation(file, readFileSync(new URL(file, dir), 'utf8'))).toEqual([]);
    });

    it('flags a quoted string containing ${...}', () => {
        expect(stringLiteralsWithInterpolation('x.ts', "const a = '<b>${html(x)}</b>';")).toEqual(['<b>${html(x)}</b>']);
    });
});
