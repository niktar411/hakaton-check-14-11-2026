// Прямое редактирование SAM: сессия, права и диалоги форм.
// Права решает сервер: без активной сессии любая мутация вернёт 401/403,
// поэтому интерфейс лишь скрывает кнопки, а не является границей доступа.
import { createEffect, createResource, createSignal, For, Show } from "solid-js";
import * as m from "@/paraglide/messages";
import { api, csrfHeaders, unwrap } from "@/shared/api";
import type { components } from "@/shared/api/schema";
import { Button } from "@/shared/ui/Button";
import { ConfirmDialog } from "@/shared/ui/ConfirmDialog";
import { Dialog } from "@/shared/ui/Dialog";
import { Input } from "@/shared/ui/Input";
import { Select, type SelectOption } from "@/shared/ui/Select";
import { A } from "@solidjs/router";
import { useToast } from "@/shared/ui/Toast";


type ProductInput = components["schemas"]["ProductInput"];
type LicenseInput = components["schemas"]["LicenseInput"];
type InstallationInput = components["schemas"]["InstallationInput"];
type Product = components["schemas"]["Product"];

// Гейт редактирования: кнопки видны всем, но без сессии открывают
// диалог со входом. Сигнал модульный — один диалог на страницу.
const [loginOpen, setLoginOpen] = createSignal(false);

export function requestEdit(loggedIn: boolean) {
	if (loggedIn) return true;
	setLoginOpen(true);
	return false;
}

type ProductStatus = NonNullable<ProductInput["status"]>;
type LicenseMetric = NonNullable<ProductInput["licenseMetric"]>;
type LicenseKind = NonNullable<LicenseInput["kind"]>;
type InstallationStatus = NonNullable<InstallationInput["status"]>;

// useSamSession возвращает сессию и CSRF-токен для мутаций. Аноним —
// нормальное состояние читателя: 401 превращается в null, а не в ошибку.
export function useSamSession() {
	const [session] = createResource(async () => {
		try {
			return unwrap(await api.GET("/api/me"));
		} catch {
			return null;
		}
	});
	const loggedIn = () => Boolean(session());
	const csrf = () => (session() as { csrf?: string } | null)?.csrf;
	return { session, loggedIn, csrf };
}

// SamEditActions — кнопки «Изменить/Удалить» в строке. Без сессии клик
// открывает диалог входа вместо правки.
export function SamEditActions(props: {
	loggedIn: boolean;
	editLabel: string;
	deleteLabel: string;
	onEdit: () => void;
	onDelete: () => void;
}) {
	return (
		<div class="flex flex-none items-center gap-2">
			<Button
				variant="secondary"
				onClick={() => {
					if (!requestEdit(props.loggedIn)) return;
					props.onEdit();
				}}
			>
				{props.editLabel}
			</Button>
			<Button
				variant="danger"
				onClick={() => {
					if (!requestEdit(props.loggedIn)) return;
					props.onDelete();
				}}
			>
				{props.deleteLabel}
			</Button>
		</div>
	);
}

// SamEditGate — один на страницу: показывает диалог входа по requestEdit.
export function SamEditGate() {
	return (
		<Show when={loginOpen()}>
			<Dialog
				open
				title={m.samEditLoginTitle()}
				onClose={() => setLoginOpen(false)}
			>
				<div class="grid min-w-0 gap-4">
					<p class="text-muted">{m.samEditLoginHint()}</p>
					<div class="flex flex-wrap justify-end gap-3">
						<Button onClick={() => setLoginOpen(false)}>{m.cancel()}</Button>
						<A class="no-underline" href="/coding">
							<Button variant="primary">{m.localLogin()}</Button>
						</A>
					</div>
				</div>
			</Dialog>
		</Show>
	);
}

// SamLoginHint — подсказка для анонимных читателей.
export function SamLoginHint(props: { loggedIn: boolean }) {
	return (
		<Show when={!props.loggedIn}>
			<p class="text-muted text-sm">{m.samEditLoginHint()}</p>
		</Show>
	);
}

export function productStatusOptions(): SelectOption[] {
	return [
		{ value: "active", label: m.samProductStatusActive() },
		{ value: "outdated", label: m.samProductStatusOutdated() },
		{ value: "banned", label: m.samProductStatusBanned() },
		{ value: "archived", label: m.samProductStatusArchived() },
	];
}

export function productMetricOptions(): SelectOption[] {
	return [
		{ value: "device", label: m.samLicenseMetricDevice() },
		{ value: "user", label: m.samLicenseMetricUser() },
		{ value: "installation", label: m.samLicenseMetricInstallation() },
		{ value: "none", label: m.samLicenseMetricNone() },
	];
}

export function licenseKindOptions(): SelectOption[] {
	return [
		{ value: "subscription", label: m.samKindSubscription() },
		{ value: "perpetual", label: m.samKindPerpetual() },
		{ value: "open_source", label: m.samKindOpenSource() },
		{ value: "trial", label: m.samKindTrial() },
	];
}

export function yesNoOptions(): SelectOption[] {
	return [
		{ value: "yes", label: m.samYes() },
		{ value: "no", label: m.samNo() },
	];
}

function pick(options: SelectOption[], value: string) {
	return options.find((option) => option.value === value) ?? options[0];
}

// ProductDialog — создание и правка продукта каталога.
export function ProductDialog(props: {
	open: boolean;
	title: string;
	value?: Product;
	pending: boolean;
	onClose: () => void;
	onSubmit: (input: ProductInput) => Promise<void>;
}) {
	const [name, setName] = createSignal("");
	const [vendor, setVendor] = createSignal("");
	const [version, setVersion] = createSignal("");
	const [category, setCategory] = createSignal("");
	const [status, setStatus] = createSignal<ProductStatus>("active");
	const [metric, setMetric] = createSignal<LicenseMetric>("device");
	const [registry, setRegistry] = createSignal("no");
	const [entry, setEntry] = createSignal("");
	const [trackable, setTrackable] = createSignal("yes");

	createEffect(() => {
		if (!props.open) return;
		const value = props.value;
		setName(value?.name ?? "");
		setVendor(value?.vendor ?? "");
		setVersion(value?.version ?? "");
		setCategory(value?.category ?? "");
		setStatus(
			pick(productStatusOptions(), value?.status ?? "active")
				.value as ProductStatus,
		);
		setMetric(
			pick(productMetricOptions(), value?.licenseMetric ?? "device")
				.value as LicenseMetric,
		);
		setRegistry(value?.registryRf ? "yes" : "no");
		setEntry(value?.registryEntry ?? "");
		setTrackable(value?.isTrackable === false ? "no" : "yes");
	});

	const submit = async (event: SubmitEvent) => {
		event.preventDefault();
		if (props.pending) return;
		await props.onSubmit({
			name: name().trim(),
			vendor: vendor().trim(),
			version: version().trim(),
			category: category().trim(),
			status: status(),
			sectionCode: props.value?.sectionCode ?? "",
			classCode: props.value?.classCode ?? "",
			licenseMetric: metric(),
			registryRf: registry() === "yes",
			registryEntry: entry().trim(),
			isTrackable: trackable() === "yes",
		});
	};

	return (
		<Dialog open={props.open} title={props.title} onClose={props.onClose}>
			<form class="grid min-w-0 gap-4" onSubmit={submit}>
				<Input
					label={m.samEditFieldProduct()}
					name="name"
					required
					value={name()}
					onInput={(event) => setName(event.currentTarget.value)}
				/>
				<Input
					label={m.samEditFieldVendor()}
					name="vendor"
					value={vendor()}
					onInput={(event) => setVendor(event.currentTarget.value)}
				/>
				<Input
					label={m.samEditFieldVersion()}
					name="version"
					value={version()}
					onInput={(event) => setVersion(event.currentTarget.value)}
				/>
				<Input
					label={m.samEditFieldCategory()}
					name="category"
					value={category()}
					onInput={(event) => setCategory(event.currentTarget.value)}
				/>
				<div class="grid gap-2">
					<span class="text-muted text-xs">{m.samEditFieldStatus()}</span>
					<Select
						label={m.samEditFieldStatus()}
						value={status()}
						onChange={(value) => setStatus(value as ProductStatus)}
						options={productStatusOptions()}
					/>
				</div>
				<div class="grid gap-2">
					<span class="text-muted text-xs">{m.samEditFieldMetric()}</span>
					<Select
						label={m.samEditFieldMetric()}
						value={metric()}
						onChange={(value) => setMetric(value as LicenseMetric)}
						options={productMetricOptions()}
					/>
				</div>
				<div class="grid gap-2">
					<span class="text-muted text-xs">{m.samEditFieldRegistry()}</span>
					<Select
						label={m.samEditFieldRegistry()}
						value={registry()}
						onChange={setRegistry}
						options={yesNoOptions()}
					/>
				</div>
				<Input
					label={m.samEditFieldRegistryEntry()}
					name="registryEntry"
					value={entry()}
					onInput={(event) => setEntry(event.currentTarget.value)}
				/>
				<div class="grid gap-2">
					<span class="text-muted text-xs">{m.samEditFieldTrackable()}</span>
					<Select
						label={m.samEditFieldTrackable()}
						value={trackable()}
						onChange={setTrackable}
						options={yesNoOptions()}
					/>
				</div>
				<div class="flex flex-wrap justify-end gap-3">
					<Button disabled={props.pending} onClick={props.onClose}>
						{m.cancel()}
					</Button>
					<Button
						variant="primary"
						type="submit"
						disabled={props.pending || !name().trim()}
					>
						{m.save()}
					</Button>
				</div>
			</form>
		</Dialog>
	);
}

// LicenseDialog — создание и правка лицензии.
export function LicenseDialog(props: {
	open: boolean;
	title: string;
	productOptions: SelectOption[];
	value?: {
		productId?: number;
		kind?: string;
		seatsTotal?: number;
		contract?: string;
		validUntil?: string | null;
	};
	pending: boolean;
	onClose: () => void;
	onSubmit: (input: LicenseInput) => Promise<void>;
}) {
	const [product, setProduct] = createSignal("");
	const [kind, setKind] = createSignal<LicenseKind>("subscription");
	const [seats, setSeats] = createSignal("0");
	const [contract, setContract] = createSignal("");
	const [validUntil, setValidUntil] = createSignal("");

	createEffect(() => {
		if (!props.open) return;
		const value = props.value;
		setProduct(value?.productId ? String(value.productId) : props.productOptions[0]?.value ?? "");
		setKind(
			pick(licenseKindOptions(), value?.kind ?? "subscription")
				.value as LicenseKind,
		);
		setSeats(String(value?.seatsTotal ?? 0));
		setContract(value?.contract ?? "");
		setValidUntil(value?.validUntil ?? "");
	});

	const submit = async (event: SubmitEvent) => {
		event.preventDefault();
		if (props.pending || !product()) return;
		await props.onSubmit({
			productId: Number(product()),
			kind: kind(),
			seatsTotal: Number(seats()) || 0,
			contract: contract().trim(),
			validUntil: validUntil() ? validUntil() : null,
		});
	};

	return (
		<Dialog open={props.open} title={props.title} onClose={props.onClose}>
			<form class="grid min-w-0 gap-4" onSubmit={submit}>
				<div class="grid gap-2">
					<span class="text-muted text-xs">{m.samEditFieldProduct()}</span>
					<Select
						label={m.samEditFieldProduct()}
						value={product()}
						onChange={setProduct}
						options={props.productOptions}
					/>
				</div>
				<div class="grid gap-2">
					<span class="text-muted text-xs">{m.samEditFieldKind()}</span>
					<Select
						label={m.samEditFieldKind()}
						value={kind()}
						onChange={(value) => setKind(value as LicenseKind)}
						options={licenseKindOptions()}
					/>
				</div>
				<Input
					label={m.samEditFieldSeats()}
					name="seatsTotal"
					type="number"
					min={0}
					required
					value={seats()}
					onInput={(event) => setSeats(event.currentTarget.value)}
				/>
				<Input
					label={m.samEditFieldContract()}
					name="contract"
					value={contract()}
					onInput={(event) => setContract(event.currentTarget.value)}
				/>
				<Input
					label={m.samEditFieldValidUntil()}
					name="validUntil"
					type="date"
					value={validUntil()}
					onInput={(event) => setValidUntil(event.currentTarget.value)}
				/>
				<div class="flex flex-wrap justify-end gap-3">
					<Button disabled={props.pending} onClick={props.onClose}>
						{m.cancel()}
					</Button>
					<Button variant="primary" type="submit" disabled={props.pending}>
						{m.save()}
					</Button>
				</div>
			</form>
		</Dialog>
	);
}

// InstallationDialog — ручная запись установки на устройстве.
export function InstallationDialog(props: {
	open: boolean;
	title: string;
	productOptions: SelectOption[];
	deviceName: string;
	value?: components["schemas"]["Installation"];
	pending: boolean;
	onClose: () => void;
	onSubmit: (input: InstallationInput) => Promise<void>;
}) {
	const [rawName, setRawName] = createSignal("");
	const [version, setVersion] = createSignal("");
	const [userName, setUserName] = createSignal("");
	const [status, setStatus] = createSignal<InstallationStatus>("installed");
	const [product, setProduct] = createSignal("");

	createEffect(() => {
		if (!props.open) return;
		const value = props.value;
		setRawName(value?.rawName ?? "");
		setVersion(value?.version ?? "");
		setUserName(value?.userName ?? "");
		setStatus(
			pick(installationStatusOptions(), value?.status ?? "installed")
				.value as InstallationStatus,
		);
		setProduct(value?.productId ? String(value.productId) : "");
	});

	function installationStatusOptions(): SelectOption[] {
		return [
			{ value: "installed", label: m.samStateOk() },
			{ value: "removed", label: m.samEditInstallationRemoved() },
			{ value: "unknown", label: m.samNotSpecified() },
		];
	}

	const submit = async (event: SubmitEvent) => {
		event.preventDefault();
		if (props.pending || !rawName().trim()) return;
		await props.onSubmit({
			rawName: rawName().trim(),
			deviceName: props.deviceName,
			userName: userName().trim(),
			version: version().trim(),
			productId: product() ? Number(product()) : null,
			status: status(),
		});
	};

	return (
		<Dialog open={props.open} title={props.title} onClose={props.onClose}>
			<form class="grid min-w-0 gap-4" onSubmit={submit}>
				<div class="grid gap-2">
					<span class="text-muted text-xs">{m.samEditFieldDevice()}</span>
					<p class="text-sm">{props.deviceName}</p>
				</div>
				<Input
					label={m.samEditFieldRawName()}
					name="rawName"
					required
					value={rawName()}
					onInput={(event) => setRawName(event.currentTarget.value)}
				/>
				<Input
					label={m.samEditFieldVersion()}
					name="version"
					value={version()}
					onInput={(event) => setVersion(event.currentTarget.value)}
				/>
				<Input
					label={m.samEditFieldUser()}
					name="userName"
					value={userName()}
					onInput={(event) => setUserName(event.currentTarget.value)}
				/>
				<div class="grid gap-2">
					<span class="text-muted text-xs">{m.samEditFieldStatus()}</span>
					<Select
						label={m.samEditFieldStatus()}
						value={status()}
						onChange={(value) => setStatus(value as InstallationStatus)}
						options={installationStatusOptions()}
					/>
				</div>
				<div class="grid gap-2">
					<span class="text-muted text-xs">{m.samEditFieldProduct()}</span>
					<Select
						label={m.samEditFieldProduct()}
						value={product()}
						onChange={setProduct}
						options={[{ value: "", label: m.samNotSpecified() }, ...props.productOptions]}
					/>
				</div>
				<div class="flex flex-wrap justify-end gap-3">
					<Button disabled={props.pending} onClick={props.onClose}>
						{m.cancel()}
					</Button>
					<Button
						variant="primary"
						type="submit"
						disabled={props.pending || !rawName().trim()}
					>
						{m.save()}
					</Button>
				</div>
			</form>
		</Dialog>
	);
}

// SamDeleteConfirm — подтверждение удаления с пояснением об аудите.
export function SamDeleteConfirm(props: {
	open: boolean;
	title: string;
	pending: boolean;
	onConfirm: () => Promise<void>;
	onClose: () => void;
}) {
	return (
		<ConfirmDialog
			open={props.open}
			title={props.title}
			description={m.samEditDeleteConfirm()}
			danger
			pending={props.pending}
			onConfirm={() => void props.onConfirm()}
			onClose={props.onClose}
		/>
	);
}

// SamAliasManager — управление алиасами продукта (страница продукта).
export function SamAliasManager(props: {
	loggedIn: boolean;
	csrf?: string;
	productID: number;
	aliases: Array<{ id: number; alias: string }>;
	onChange: () => void;
}) {
	const toast = useToast();
	const [alias, setAlias] = createSignal("");
	const [pending, setPending] = createSignal(false);

	async function add(event: SubmitEvent) {
		event.preventDefault();
		if (pending() || !alias().trim() || !requestEdit(props.loggedIn)) return;
		setPending(true);
		try {
			await api.POST("/api/sam/products/{id}/aliases", {
				params: { path: { id: props.productID } },
				body: { alias: alias().trim() },
				headers: csrfHeaders(props.csrf),
			});
			setAlias("");
			props.onChange();
			toast.show(m.samEditSaved());
		} catch {
			toast.show(m.samEditFailed());
		} finally {
			setPending(false);
		}
	}

	async function remove(id: number) {
		if (pending()) return;
		setPending(true);
		try {
			await api.DELETE("/api/sam/aliases/{id}", {
				params: { path: { id } },
				headers: csrfHeaders(props.csrf),
			});
			props.onChange();
			toast.show(m.samEditSaved());
		} catch {
			toast.show(m.samEditFailed());
		} finally {
			setPending(false);
		}
	}

	return (
		<Show when={props.loggedIn}>
			<form class="flex flex-wrap items-end gap-3" onSubmit={add}>
				<Input
					label={m.samEditFieldAlias()}
					name="alias"
					value={alias()}
					onInput={(event) => setAlias(event.currentTarget.value)}
				/>
				<Button variant="secondary" type="submit" disabled={pending() || !alias().trim()}>
					{m.samEditAddAlias()}
				</Button>
			</form>
			<ul class="flex flex-wrap gap-2">
				<For each={props.aliases}>
					{(item) => (
						<li class="border-border bg-surface-2 flex items-center gap-2 rounded-full border px-3 py-1 text-sm">
							{item.alias}
							<Button
								variant="danger"
								aria-label={`${m.delete()}: ${item.alias}`}
								disabled={pending()}
								onClick={() => void remove(item.id)}
							>
								×
							</Button>
						</li>
					)}
				</For>
			</ul>
		</Show>
	);
}
