import type { JSX } from "solid-js";
import * as m from "@/paraglide/messages";
import s from "./LegalPage.module.css";

type Section = {
	heading: () => string;
	body: () => string;
};

function LegalPage(props: {
	title: string;
	intro: string;
	sections: Section[];
	children?: JSX.Element;
}) {
	return (
		<main class={s.page}>
			<article class={s.inner}>
			<header class={s.header}>
				<p class={s.eyebrow}>{m.legalUpdated()}</p>
				<h1 class={s.title}>{props.title}</h1>
				<p class={s.intro}>{props.intro}</p>
			</header>
			{props.sections.map((section) => (
				<section class={s.section}>
					<h2 class={s.heading}>{section.heading()}</h2>
					<p class={s.body}>{section.body()}</p>
				</section>
			))}
			<footer class={s.footer}>
				<p>{m.legalContact()}</p>
			</footer>
			</article>
		</main>
	);
}

export function TermsPage() {
	return (
		<LegalPage
			title={m.legalTermsTitle()}
			intro={m.legalTermsIntro()}
			sections={[
				{ heading: () => m.legalTermsS1(), body: () => m.legalTermsB1() },
				{ heading: () => m.legalTermsS2(), body: () => m.legalTermsB2() },
				{ heading: () => m.legalTermsS3(), body: () => m.legalTermsB3() },
				{ heading: () => m.legalTermsS4(), body: () => m.legalTermsB4() },
				{ heading: () => m.legalTermsS5(), body: () => m.legalTermsB5() },
				{ heading: () => m.legalTermsS6(), body: () => m.legalTermsB6() },
				{ heading: () => m.legalTermsS7(), body: () => m.legalTermsB7() },
				{ heading: () => m.legalTermsS8(), body: () => m.legalTermsB8() },
			]}
		/>
	);
}

export function PolicyPage() {
	return (
		<LegalPage
			title={m.legalPolicyTitle()}
			intro={m.legalPolicyIntro()}
			sections={[
				{ heading: () => m.legalPolicyS1(), body: () => m.legalPolicyB1() },
				{ heading: () => m.legalPolicyS2(), body: () => m.legalPolicyB2() },
				{ heading: () => m.legalPolicyS3(), body: () => m.legalPolicyB3() },
				{ heading: () => m.legalPolicyS4(), body: () => m.legalPolicyB4() },
				{ heading: () => m.legalPolicyS5(), body: () => m.legalPolicyB5() },
				{ heading: () => m.legalPolicyS6(), body: () => m.legalPolicyB6() },
				{ heading: () => m.legalPolicyS7(), body: () => m.legalPolicyB7() },
				{ heading: () => m.legalPolicyS8(), body: () => m.legalPolicyB8() },
				{ heading: () => m.legalPolicyS9(), body: () => m.legalPolicyB9() },
				{ heading: () => m.legalPolicyS10(), body: () => m.legalPolicyB10() },
				{ heading: () => m.legalPolicyS11(), body: () => m.legalPolicyB11() },
			]}
		/>
	);
}

export function ConsentPage() {
	return (
		<LegalPage
			title={m.legalConsentTitle()}
			intro={m.legalConsentIntro()}
			sections={[
				{ heading: () => m.legalConsentS1(), body: () => m.legalConsentB1() },
				{ heading: () => m.legalConsentS2(), body: () => m.legalConsentB2() },
				{ heading: () => m.legalConsentS3(), body: () => m.legalConsentB3() },
				{ heading: () => m.legalConsentS4(), body: () => m.legalConsentB4() },
				{ heading: () => m.legalConsentS5(), body: () => m.legalConsentB5() },
			]}
		/>
	);
}

export function CookiesPage() {
	return (
		<LegalPage
			title={m.legalCookiesTitle()}
			intro={m.legalCookiesIntro()}
			sections={[
				{ heading: () => m.legalCookiesS1(), body: () => m.legalCookiesB1() },
				{ heading: () => m.legalCookiesS2(), body: () => m.legalCookiesB2() },
				{ heading: () => m.legalCookiesS3(), body: () => m.legalCookiesB3() },
				{ heading: () => m.legalCookiesS4(), body: () => m.legalCookiesB4() },
			]}
		/>
	);
}
