import * as m from "@/paraglide/messages";
import s from "./ChatPage.module.css";

export function ChatModeTabs(props: {
	agent: boolean;
	onChange: (agent: boolean) => void;
}) {
	return (
		<div class={s.modeTabs} role="tablist" aria-label={m.chatMode()}>
			<button
				type="button"
				role="tab"
				aria-selected={!props.agent}
				data-active={!props.agent}
				onClick={() => props.onChange(false)}
			>
				{m.chatModeChat()}
			</button>
			<button
				type="button"
				role="tab"
				aria-selected={props.agent}
				data-active={props.agent}
				onClick={() => props.onChange(true)}
			>
				{m.chatModeAgent()}
			</button>
		</div>
	);
}
