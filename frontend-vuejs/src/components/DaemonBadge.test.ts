import {describe, it, expect} from 'vitest';
import {mount} from '@vue/test-utils';
import DaemonBadge from './DaemonBadge.vue';

describe('DaemonBadge', () => {
    it.each([
        ['running', 'daemon-running'],
        ['stopped', 'daemon-stopped'],
        ['not installed', 'daemon-not-installed'],
        ['something else', 'daemon-unknown'],
    ])('renders %s with the %s modifier', (status, modifier) => {
        const badge = mount(DaemonBadge, {props: {status}}).find('.home-badge');
        expect(badge.classes()).toContain(modifier);
        expect(badge.text()).toBe(status);
    });

    it('escapes HTML in the status', () => {
        const wrapper = mount(DaemonBadge, {props: {status: '<b>x</b>'}});
        expect(wrapper.find('b').exists()).toBe(false);
        expect(wrapper.text()).toBe('<b>x</b>');
    });
});
