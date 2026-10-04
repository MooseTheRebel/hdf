import {defineComponent, h, Fragment} from 'vue';

// I18nText renders a message whose {name} placeholders are filled by the
// same-named slots, so markup stays out of the locale files:
//
//   <I18nText :template="msg.config.missing">
//       <template #command><code>{{ msg.config.missingCommand }}</code></template>
//   </I18nText>
//
// The message text itself is rendered as text (escaped); a placeholder
// with no matching slot is left as-is.
export default defineComponent({
    name: 'I18nText',
    props: {
        template: {type: String, required: true},
    },
    setup(props, {slots}) {
        return () => h(Fragment, props.template.split(/(\{\w+\})/).map((part) => {
            const slot = /^\{(\w+)\}$/.exec(part)?.[1];
            const fill = slot ? slots[slot] : undefined;
            return fill ? h(Fragment, fill()) : part;
        }));
    },
});
