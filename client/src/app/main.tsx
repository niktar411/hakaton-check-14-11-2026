import { A, Route, Router, useLocation } from "@solidjs/router";
import { createEffect, createResource, type JSX, Show } from "solid-js";
import { render } from "solid-js/web";
import "@/shared/styles/app.css";
import * as m from "@/paraglide/messages";
import { api, unwrap } from "@/shared/api";
import { Footer } from "@/shared/ui/Footer";
import { Toast } from "@/shared/ui/Toast";
import LandingPage from "@/features/public/LandingPage";
import QuestionsPage from "@/features/questions/QuestionsPage";
import PlannerPage from "@/features/planner/PlannerPage";
import CodingPage from "@/features/coding/CodingPage";
import AdminLoginPage from "@/features/administration/AdminLoginPage";
import AdministrationPage from "@/features/administration/AdministrationPage";
import { SamLayout } from "@/features/sam/SamLayout";
import SamDashboardPage from "@/features/sam/SamDashboardPage";
import SamCatalogPage from "@/features/sam/SamCatalogPage";
import SamLicensesPage from "@/features/sam/SamLicensesPage";
import SamInventoryPage from "@/features/sam/SamInventoryPage";
import SamCompliancePage from "@/features/sam/SamCompliancePage";
import SamProductPage from "@/features/sam/SamProductPage";
import SamHostPage from "@/features/sam/SamHostPage";
import SamMatchingPage from "@/features/sam/SamMatchingPage";
import SamAssistantPage from "@/features/sam/SamAssistantPage";
import { SiteConfigProvider } from "@/app/site-config";

function isSamPath(pathname: string) {
	return pathname === "/sam" || pathname.startsWith("/sam/");
}

// Set the theme before the first render so a direct /sam visit does not flash
// the light palette while Solid mounts.
document.documentElement.dataset.theme = isSamPath(window.location.pathname)
	? "dark"
	: "light";

function PublicNavigation(props: { publicUrl?: string }) {
	return (
		<nav class="flex flex-wrap gap-x-5 gap-y-2" aria-label={m.mainNavigation()}>
			<a href={props.publicUrl ?? "/"}>{m.footerHome()}</a>
			<A href="/questions">{m.questionsPageTitle()}</A>
			<A href="/planner">{m.plannerPageTitle()}</A>
			<A href="/sam">{m.samNavigation()}</A>
		</nav>
	);
}

function App(props: { children?: JSX.Element }) {
	const location = useLocation();
	const [config, { refetch }] = createResource(async () =>
		unwrap(await api.GET("/api/public/site-config")),
	);
	const adminArea = () =>
		location.pathname === "/admin" || location.pathname.startsWith("/admin/");
	const publicAdminLogin = () => location.pathname === "/admin/login";
	const privateArea = () =>
		adminArea() ||
		location.pathname === "/coding" ||
		location.pathname.startsWith("/coding/");
	const samArea = () => isSamPath(location.pathname);
	createEffect(() => {
		const robots = document.querySelector('meta[name="robots"]');
		if (robots) {
			robots.setAttribute(
				"content",
				privateArea() ? "noindex,nofollow" : "index,follow",
			);
		}
	});
	createEffect(() => {
		document.documentElement.dataset.theme = samArea() ? "dark" : "light";
	});
	return (
		<SiteConfigProvider
			value={{
				config,
				retry: () => {
					void refetch();
				},
			}}
		>
			<Toast />
			<Show
				when={!adminArea() || publicAdminLogin()}
				fallback={
					<div class="mx-auto flex min-h-dvh w-full max-w-7xl flex-col gap-6 p-4 md:p-8">
						<header class="border-border flex flex-wrap items-center justify-between gap-4 border-b pb-4">
							<A class="text-text text-xl font-bold no-underline" href="/admin">
								{m.administrationPageTitle()}
							</A>
							<A href="/">{m.publicSite()}</A>
						</header>
						{props.children}
					</div>
				}
			>
				<div class="mx-auto flex min-h-dvh w-full max-w-6xl flex-col gap-6 p-4 md:p-8">
					<header class="border-border flex flex-wrap items-center justify-between gap-4 border-b pb-4">
						<A class="text-text text-xl font-bold no-underline" href="/">
							{m.appTitle()}
						</A>
						<PublicNavigation
							publicUrl={config.error ? undefined : config()?.publicBaseUrl}
						/>
					</header>
					<div class="flex flex-1 flex-col">{props.children}</div>
					<Footer
						publicUrl={config.error ? undefined : config()?.publicBaseUrl}
						plannerUrl={config.error ? undefined : config()?.plannerBaseUrl}
					/>
				</div>
			</Show>
		</SiteConfigProvider>
	);
}

const root = document.getElementById("root");
if (root) {
	render(
		() => (
			<Router root={App}>
				<Route path="/" component={LandingPage} />
				<Route path="/questions" component={QuestionsPage} />
				<Route path="/planner" component={PlannerPage} />
				<Route path="/sam" component={SamLayout}>
					<Route path="/" component={SamDashboardPage} />
					<Route path="/catalog" component={SamCatalogPage} />
					<Route path="/catalog/:id" component={SamProductPage} />
					<Route path="/licenses" component={SamLicensesPage} />
					<Route path="/inventory" component={SamInventoryPage} />
					<Route path="/inventory/:id" component={SamHostPage} />
					<Route path="/compliance" component={SamCompliancePage} />
					<Route path="/matching" component={SamMatchingPage} />
					<Route path="/assistant" component={SamAssistantPage} />
				</Route>
				<Route path="/coding" component={CodingPage} />
				<Route path="/admin/login" component={AdminLoginPage} />
				<Route path="/admin" component={AdministrationPage} />
			</Router>
		),
		root,
	);
}
