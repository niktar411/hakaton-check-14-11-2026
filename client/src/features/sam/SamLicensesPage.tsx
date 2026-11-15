import { createSignal, For, Show } from "solid-js";
import * as m from "@/paraglide/messages";
import { api, csrfHeaders, unwrap } from "@/shared/api";
import type { components } from "@/shared/api/schema";
import { formatDay } from "@/shared/lib/format";
import { Button } from "@/shared/ui/Button";
import { useToast } from "@/shared/ui/Toast";
import type { SelectOption } from "@/shared/ui/Select";
import { SamBadge, SamFields, SamList, SamPageHeader } from "./SamPage";
import s from "./SamPage.module.css";
import { createSamResource, SamPageState } from "./SamPageState";
import { licenseKindLabel, licenseStatusLabel, licenseStatusTone } from "./labels";
import {
	LicenseDialog,
	requestEdit,
	SamDeleteConfirm,
	SamEditActions,
	SamEditGate,
	SamLoginHint,
	useSamSession,
} from "./edit";

type License = components["schemas"]["License"];
type LicenseInput = components["schemas"]["LicenseInput"];
type Product = components["schemas"]["Product"];

export default function SamLicensesPage() {
	const { resource: licenses, refetch } = createSamResource<License[]>(async () =>
		unwrap(await api.GET("/api/public/sam/licenses")) ?? [],
	);
	const { resource: products } = createSamResource<Product[]>(async () =>
		unwrap(await api.GET("/api/public/sam/catalog")) ?? [],
	);
	const session = useSamSession();
	const toast = useToast();

	const productOptions = (): SelectOption[] =>
		(products() ?? []).map((product) => ({
			value: String(product.id),
			label: product.name,
		}));

	const [formOpen, setFormOpen] = createSignal(false);
	const [formValue, setFormValue] = createSignal<License & { productId?: number }>();
	const [pending, setPending] = createSignal(false);
	const [deleteTarget, setDeleteTarget] = createSignal<License>();
	const [deletePending, setDeletePending] = createSignal(false);

	async function submit(input: LicenseInput) {
		setPending(true);
		try {
			const editing = formValue();
			if (editing?.id) {
				await api.PUT("/api/sam/licenses/{id}", {
					params: { path: { id: editing.id } },
					body: input,
					headers: csrfHeaders(session.csrf()),
				});
			} else {
				await api.POST("/api/sam/licenses", {
					body: input,
					headers: csrfHeaders(session.csrf()),
				});
			}
			setFormOpen(false);
			void refetch();
			toast.show(m.samEditSaved());
		} catch {
			toast.show(m.samEditFailed());
		} finally {
			setPending(false);
		}
	}

	async function remove() {
		const target = deleteTarget();
		if (!target || deletePending()) return;
		setDeletePending(true);
		try {
			await api.DELETE("/api/sam/licenses/{id}", {
				params: { path: { id: target.id } },
				headers: csrfHeaders(session.csrf()),
			});
			setDeleteTarget(undefined);
			void refetch();
			toast.show(m.samEditSaved());
		} catch {
			toast.show(m.samEditFailed());
		} finally {
			setDeletePending(false);
		}
	}

	return (
		<>
			<SamPageHeader
				title={m.samLicensesPageTitle()}
				lead={m.samLicensesLead()}
			/>
			<SamLoginHint loggedIn={session.loggedIn()} />
			<SamEditGate />
			<div class={s.addAction}>
			<Button
				variant="primary"
				onClick={() => {
					if (!requestEdit(session.loggedIn())) return;
					setFormValue(undefined);
					setFormOpen(true);
				}}
			>
				{m.samEditAddLicense()}
			</Button>
			</div>
			<SamPageState
				data={licenses}
				empty={(value: License[]) => value.length === 0}
				onRetry={() => void refetch()}
				emptyMessage={m.samLicensesEmpty()}
			>
				{(value) => (
					<SamList>
						<For each={value}>
							{(license) => (
								<li class={s.row}>
									<div>
										<div class={s.rowHeader}>
											<h2 class={s.rowTitle}>{license.product}</h2>
											<SamBadge
												label={licenseStatusLabel(license.status)}
												tone={licenseStatusTone(license.status)}
											/>
										</div>
										<p class={s.rowMeta}>{license.vendor}</p>
									</div>
									<SamFields
										wide
										items={[
											{
												label: m.samLicenseKind(),
												value: licenseKindLabel(license.kind),
											},
											{
												label: m.samLicenseSeats(),
												value: `${license.seatsUsed} / ${license.seatsTotal}`,
											},
											{
												label: m.samLicenseValidUntil(),
												value: license.validUntil
													? formatDay(license.validUntil)
													: m.samLicenseNoExpiry(),
											},
										]}
									/>
									<div class={s.rowTools}>
										<SamEditActions
											loggedIn={session.loggedIn()}
											editLabel={m.samEditEditLicense()}
											deleteLabel={m.samEditDeleteLicense()}
											onEdit={() => {
												const record = products()?.find(
													(product) => product.name === license.product,
												);
												setFormValue({
													...license,
													productId: record?.id,
												});
												setFormOpen(true);
											}}
											onDelete={() => setDeleteTarget(license)}
										/>
									</div>
								</li>
							)}
						</For>
					</SamList>
				)}
			</SamPageState>
			<LicenseDialog
				open={formOpen()}
				title={formValue()?.id ? m.samEditEditLicense() : m.samEditAddLicense()}
				productOptions={productOptions()}
				value={formValue()}
				pending={pending()}
				onClose={() => setFormOpen(false)}
				onSubmit={submit}
			/>
			<SamDeleteConfirm
				open={Boolean(deleteTarget())}
				title={`${m.samEditDeleteLicense()}: ${deleteTarget()?.product ?? ""}`}
				pending={deletePending()}
				onConfirm={remove}
				onClose={() => setDeleteTarget(undefined)}
			/>
		</>
	);
}
