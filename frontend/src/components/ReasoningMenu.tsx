import { Check, ChevronDown } from "lucide-solid";
import { For, Show, createSignal } from "solid-js";
import type { ReasoningLevel, ReasoningOption } from "../types";

type ReasoningMenuProps = {
  value: ReasoningLevel;
  options: ReasoningOption[];
  running?: boolean;
  onChange: (level: ReasoningLevel) => void;
};

export function ReasoningMenu(props: ReasoningMenuProps) {
  const [open, setOpen] = createSignal(false);

  const current = () => props.options.find((option) => option.id === props.value) ?? props.options[0];

  const select = (level: ReasoningLevel) => {
    setOpen(false);
    if (level !== props.value) {
      props.onChange(level);
    }
  };

  return (
    <div class="reasoning-menu">
      <button
        class="reasoning-trigger"
        type="button"
        aria-haspopup="menu"
        aria-expanded={open()}
        title="选择后续请求的推理强度"
        onClick={() => setOpen((value) => !value)}
      >
        <span>推理</span>
        <strong>{current().label}</strong>
        <ChevronDown size={16} aria-hidden="true" />
      </button>
      <Show when={open()}>
        <div class="reasoning-popover" role="menu">
          <For each={props.options}>
            {(option) => (
              <button
                class="reasoning-option"
                classList={{ selected: option.id === props.value }}
                type="button"
                role="menuitemradio"
                aria-checked={option.id === props.value}
                onClick={() => select(option.id)}
              >
                <span>
                  <strong>{option.label}</strong>
                  <small>{option.description}</small>
                </span>
                <Show when={option.id === props.value}>
                  <Check size={16} aria-label="已选中" />
                </Show>
              </button>
            )}
          </For>
        </div>
      </Show>
      <Show when={props.running}>
        <span class="pending-note">下一轮生效</span>
      </Show>
    </div>
  );
}
