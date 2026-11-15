import { A } from "@solidjs/router";
import { createSignal, For, Show } from "solid-js";
import * as m from "@/paraglide/messages";
import { api, csrfHeaders, unwrap } from "@/shared/api";
import type { components } from "@/shared/api/schema";
import { formatDate } from "@/shared/lib/format";
import { Button } from "@/shared/ui/Button";
import { useToast } from "@/shared/ui/Toast";
import { SamFields, SamList, SamPageHeader } from "./SamPage";
import s from "./SamPage.module.css";
import { createSamResource, SamPageState } from "./SamPageState";
import { ProductDialog as ProductFormDialog, requestEdit, SamDeleteConfirm, SamEditActions, SamEditGate, SamLoginHint, useSamSession } from "./edit";

type Product = components["schemas"]["Product"];
type ProductInput = components["schemas"]["ProductInput"];

export default function SamCatalogPage() {
	const { resource: products, refetch } = createSamResource<Product[]>(async () =>
		unwrap(await api.GET("/api/public/sam/catalog")) ?? [],
	);
	const session = useSamSession();
	const toast = useToast();
	const [formOpen, setFormOpen] = createSignal(false);
	const [formValue, setFormValue] = createSignal<Product | undefined>();
	const [pending, setPending] = createSignal(false);
	const [deleteTarget, setDeleteTarget] = createSignal<Product>();
	const [deletePending, setDeletePending] = createSignal(false);

	async function submit(input: ProductInput) {
		setPending(true);
		try {
			const editing = formValue();
			if (editing) {
				await api.PUT("/api/sam/products/{id}", {
					params: { path: { id: editing.id } },
					body: input,
					headers: csrfHeaders(session.csrf()),
				});
			} else {
				await api.POST("/api/sam/products", {
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
			await api.DELETE("/api/sam/products/{id}", {
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
				title={m.samCatalogPageTitle()}
				lead={m.samCatalogLead()}
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
				{m.samEditAddProduct()}
			</Button>
			</div>
			<SamPageState
				data={products}
				empty={(value: Product[]) => value.length === 0}
				onRetry={() => void refetch()}
				emptyMessage={m.samCatalogEmpty()}
			>
				{(value) => (
					<SamList>
						<For each={value}>
							{(product) => (
								<li class={s.row}>
									<div>
										<h2 class={s.rowTitle}>
											<A class={s.inlineLink} href={`/sam/catalog/${product.id}`}>
												{product.name}
											</A>
										</h2>
										<p class={s.rowMeta}>
											{product.vendor} · {product.category}
										</p>
									</div>
									<SamFields
										wide
										items={[
											{
												label: m.samLicenseCount(),
												value: String(product.licenseCount),
											},
											{
												label: m.samInstallationCount(),
												value: String(product.installationCount),
											},
											{
												label: m.samUpdatedAt(),
												value: formatDate(product.updatedAt),
											},
										]}
									/>
									<div class={s.rowTools}>
										<SamEditActions
											loggedIn={session.loggedIn()}
											editLabel={m.samEditEditProduct()}
											deleteLabel={m.samEditDeleteProduct()}
											onEdit={() => {
												setFormValue(product);
												setFormOpen(true);
											}}
											onDelete={() => setDeleteTarget(product)}
										/>
									</div>
								</li>
							)}
						</For>
					</SamList>
				)}
			</SamPageState>
			<ProductFormDialog
				open={formOpen()}
				title={formValue() ? m.samEditEditProduct() : m.samEditAddProduct()}
				value={formValue()}
				pending={pending()}
				onClose={() => setFormOpen(false)}
				onSubmit={submit}
			/>
			<SamDeleteConfirm
				open={Boolean(deleteTarget())}
				title={`${m.samEditDeleteProduct()}: ${deleteTarget()?.name ?? ""}`}
				pending={deletePending()}
				onConfirm={remove}
				onClose={() => setDeleteTarget(undefined)}
			/>
		</>
	);
}
