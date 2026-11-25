"use client";

import { FormEvent, useCallback, useEffect, useMemo, useState, type ReactNode } from "react";
import { ArrowLeft, ClipboardList, LoaderCircle, Plus, RefreshCw, Search, Truck, X } from "lucide-react";

const API = process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080";
type Supplier = { id: string; name: string; is_active: boolean };
type Product = { id: string; name: string; sku: string; cost_price_minor: number; is_active: boolean };
type PurchaseOrder = { id: string; po_number: string; supplier_id: string; status: string; notes?: string | null; created_at: string };
type OrderLine = { id: string; product_id: string; product_name: string; ordered_quantity: string; received_quantity: string; remaining_quantity: string; unit_cost_minor: number };
type ReceiptLine = { id: string; product_id: string; product_name: string; received_quantity: string; unit_cost_minor: number; movement_id: string };
type Receipt = { id: string; received_by: string; note?: string | null; created_at: string; lines: ReceiptLine[] };
type OrderDetail = { order: { id: string; po_number: string; status: string; supplier_name: string; notes?: string | null }; lines: OrderLine[]; receipts: Receipt[] };
type DraftLine = { product_id: string; quantity: string; unit_cost_minor: string };

async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${API}${path}`, { ...init, credentials: "include", headers: { "Content-Type": "application/json", ...(init?.headers ?? {}) } });
  const body = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(body.error ?? `Request failed (${response.status})`);
  return body as T;
}
const money = (minor: number) => new Intl.NumberFormat("en-KE", { style: "currency", currency: "KES", maximumFractionDigits: 2 }).format(minor / 100);
const date = (value: string) => new Date(value).toLocaleString("en-KE", { dateStyle: "medium", timeStyle: "short" });

export default function PurchasingPage() {
  const [orders, setOrders] = useState<PurchaseOrder[]>([]);
  const [suppliers, setSuppliers] = useState<Supplier[]>([]);
  const [products, setProducts] = useState<Product[]>([]);
  const [detail, setDetail] = useState<OrderDetail | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [search, setSearch] = useState("");
  const [showCreate, setShowCreate] = useState(false);
  const [supplierId, setSupplierId] = useState("");
  const [poNumber, setPoNumber] = useState("");
  const [notes, setNotes] = useState("");
  const [draftLines, setDraftLines] = useState<DraftLine[]>([{ product_id: "", quantity: "1", unit_cost_minor: "0" }]);
  const [receiveQty, setReceiveQty] = useState<Record<string, string>>({});
  const [receiptNote, setReceiptNote] = useState("");

  const load = useCallback(async () => {
    setLoading(true); setError("");
    try {
      const [o, s, p] = await Promise.all([
        api<{ items: PurchaseOrder[] }>("/api/v1/purchasing/orders?limit=100"),
        api<Supplier[]>("/api/v1/catalog/suppliers?limit=200"),
        api<Product[]>("/api/v1/catalog/products?limit=500"),
      ]);
      setOrders(o.items ?? []); setSuppliers(s.filter(x => x.is_active)); setProducts(p.filter(x => x.is_active));
    } catch (e) { setError(e instanceof Error ? e.message : "Unable to load purchasing data"); }
    finally { setLoading(false); }
  }, []);
  useEffect(() => { void load(); }, [load]);

  const visibleOrders = useMemo(() => orders.filter(o => `${o.po_number} ${o.status}`.toLowerCase().includes(search.toLowerCase())), [orders, search]);
  const openOrder = async (id: string) => {
    setError(""); setNotice("");
    try { const result = await api<OrderDetail>(`/api/v1/purchasing/orders/${id}`); setDetail(result); setReceiveQty(Object.fromEntries(result.lines.map(l => [l.id, l.remaining_quantity]))); }
    catch (e) { setError(e instanceof Error ? e.message : "Unable to open purchase order"); }
  };
  const updateLine = (index: number, key: keyof DraftLine, value: string) => setDraftLines(old => old.map((line, i) => i === index ? { ...line, [key]: value } : line));
  const createOrder = async (event: FormEvent) => {
    event.preventDefault(); setSaving(true); setError(""); setNotice("");
    try {
      const result = await api<{ id: string }>("/api/v1/purchasing/orders", { method: "POST", body: JSON.stringify({ supplier_id: supplierId, po_number: poNumber.trim(), notes, lines: draftLines.map(l => ({ product_id: l.product_id, quantity: l.quantity, unit_cost_minor: Number(l.unit_cost_minor) })) }) });
      setShowCreate(false); setPoNumber(""); setNotes(""); setDraftLines([{ product_id: "", quantity: "1", unit_cost_minor: "0" }]);
      await load(); await openOrder(result.id); setNotice("Purchase order created as a draft.");
    } catch (e) { setError(e instanceof Error ? e.message : "Could not create purchase order"); }
    finally { setSaving(false); }
  };
  const transition = async (action: "order" | "cancel") => {
    if (!detail) return;
    const message = action === "cancel" ? "Cancel this purchase order? This cannot be undone." : "Mark this purchase order as ordered?";
    if (!window.confirm(message)) return;
    setSaving(true); setError("");
    try { await api(`/api/v1/purchasing/orders/${detail.order.id}/${action}`, { method: "POST", body: "{}" }); await load(); await openOrder(detail.order.id); setNotice(action === "cancel" ? "Purchase order cancelled." : "Purchase order marked as ordered."); }
    catch (e) { setError(e instanceof Error ? e.message : `Could not ${action} purchase order`); }
    finally { setSaving(false); }
  };
  const receive = async () => {
    if (!detail) return;
    const lines = detail.lines.map(l => ({ purchase_order_line_id: l.id, quantity: (receiveQty[l.id] ?? "0").trim() })).filter(l => Number(l.quantity) > 0);
    if (!lines.length) { setError("Enter at least one quantity to receive."); return; }
    setSaving(true); setError("");
    try {
      await api(`/api/v1/purchasing/orders/${detail.order.id}/receive`, { method: "POST", headers: { "Idempotency-Key": crypto.randomUUID() }, body: JSON.stringify({ note: receiptNote, lines }) });
      setReceiptNote(""); await load(); await openOrder(detail.order.id); setNotice("Goods received and inventory movements recorded.");
    } catch (e) { setError(e instanceof Error ? e.message : "Goods could not be received"); }
    finally { setSaving(false); }
  };

  return <main className="min-h-screen bg-[#f6f5f2] text-stone-900">
    <header className="border-b border-stone-200 bg-white"><div className="mx-auto flex max-w-7xl items-center justify-between px-5 py-4"><a href="/app" className="text-sm font-semibold tracking-wide text-amber-700">MEKU-In <span className="font-normal text-stone-400">/ Operations</span></a><a href="/app" className="text-sm text-stone-500 hover:text-stone-900">Back to workspace</a></div></header>
    <div className="mx-auto max-w-7xl px-5 py-8">
      <div className="flex flex-wrap items-end justify-between gap-4"><div><p className="text-xs font-semibold uppercase tracking-[.18em] text-amber-700">Stock & procurement</p><h1 className="mt-2 text-3xl font-semibold tracking-tight">Purchase orders</h1><p className="mt-2 text-sm text-stone-500">Create orders, track supplier deliveries and record received stock.</p></div><div className="flex gap-2"><button onClick={() => void load()} className="inline-flex items-center gap-2 rounded-lg border border-stone-300 bg-white px-4 py-2.5 text-sm font-medium hover:bg-stone-50"><RefreshCw size={16}/> Refresh</button><button onClick={() => setShowCreate(true)} className="inline-flex items-center gap-2 rounded-lg bg-stone-900 px-4 py-2.5 text-sm font-medium text-white hover:bg-stone-700"><Plus size={17}/> New purchase order</button></div></div>
      {error && <p role="alert" className="mt-5 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800">{error}</p>}{notice && <p role="status" className="mt-5 rounded-lg border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-800">{notice}</p>}
      <div className="mt-7 grid gap-4 sm:grid-cols-3"><Stat label="Purchase orders" value={orders.length}/><Stat label="Awaiting delivery" value={orders.filter(o => o.status === "ordered" || o.status === "partially_received").length}/><Stat label="Completed / closed" value={orders.filter(o => o.status === "received" || o.status === "cancelled").length}/></div>
      <section className="mt-7 overflow-hidden rounded-2xl border border-stone-200 bg-white shadow-sm"><div className="flex flex-wrap items-center justify-between gap-3 border-b border-stone-100 p-4"><h2 className="font-semibold">Order register</h2><label className="flex items-center gap-2 rounded-lg border border-stone-200 px-3 py-2"><Search size={16} className="text-stone-400"/><input value={search} onChange={e => setSearch(e.target.value)} placeholder="Search PO number or status" className="w-56 text-sm outline-none"/></label></div>
        {loading ? <div className="flex items-center justify-center gap-2 p-12 text-sm text-stone-500"><LoaderCircle className="animate-spin" size={18}/> Loading purchase orders</div> : visibleOrders.length === 0 ? <div className="p-12 text-center"><ClipboardList className="mx-auto text-stone-300" size={32}/><p className="mt-3 font-medium">No purchase orders found</p><p className="mt-1 text-sm text-stone-500">Create a purchase order to begin tracking a supplier delivery.</p></div> : <div className="overflow-x-auto"><table className="w-full text-left text-sm"><thead className="bg-stone-50 text-xs uppercase tracking-wide text-stone-500"><tr><th className="px-5 py-3">PO number</th><th className="px-5 py-3">Supplier</th><th className="px-5 py-3">Status</th><th className="px-5 py-3">Created</th><th className="px-5 py-3"></th></tr></thead><tbody className="divide-y divide-stone-100">{visibleOrders.map(o => <tr key={o.id} className="hover:bg-stone-50"><td className="px-5 py-4 font-semibold">{o.po_number}</td><td className="px-5 py-4 text-stone-600">{suppliers.find(s => s.id === o.supplier_id)?.name ?? "Supplier"}</td><td className="px-5 py-4"><Status value={o.status}/></td><td className="px-5 py-4 text-stone-500">{date(o.created_at)}</td><td className="px-5 py-4 text-right"><button onClick={() => void openOrder(o.id)} className="font-medium text-amber-800 hover:underline">View details</button></td></tr>)}</tbody></table></div>}
      </section>
    </div>

    {showCreate && <div className="fixed inset-0 z-40 flex items-start justify-center overflow-y-auto bg-black/40 p-4"><form onSubmit={createOrder} className="my-8 w-full max-w-3xl rounded-2xl bg-white p-6 shadow-xl"><div className="flex items-center justify-between"><div><p className="text-xs font-semibold uppercase tracking-widest text-amber-700">Procurement</p><h2 className="mt-1 text-xl font-semibold">Create purchase order</h2></div><button type="button" onClick={() => setShowCreate(false)} className="rounded-lg p-2 hover:bg-stone-100"><X size={18}/></button></div><div className="mt-6 grid gap-4 sm:grid-cols-2"><Field label="PO number"><input required maxLength={80} value={poNumber} onChange={e => setPoNumber(e.target.value)} className="field" placeholder="e.g. PO-2026-001"/></Field><Field label="Supplier"><select required value={supplierId} onChange={e => setSupplierId(e.target.value)} className="field"><option value="">Select supplier</option>{suppliers.map(s => <option key={s.id} value={s.id}>{s.name}</option>)}</select></Field></div><div className="mt-6 flex items-center justify-between"><h3 className="font-semibold">Order lines</h3><button type="button" onClick={() => setDraftLines(old => [...old, { product_id: "", quantity: "1", unit_cost_minor: "0" }])} className="inline-flex items-center gap-1 text-sm font-medium text-amber-800"><Plus size={15}/> Add line</button></div><div className="mt-3 space-y-3">{draftLines.map((line, i) => <div key={i} className="grid gap-3 rounded-xl border border-stone-200 p-3 sm:grid-cols-[1fr_120px_150px_32px]"><Field label="Product"><select required value={line.product_id} onChange={e => updateLine(i,"product_id",e.target.value)} className="field"><option value="">Select product</option>{products.map(p => <option key={p.id} value={p.id}>{p.name} · {p.sku}</option>)}</select></Field><Field label="Quantity"><input required pattern="[0-9]+(\\.[0-9]{1,4})?" value={line.quantity} onChange={e => updateLine(i,"quantity",e.target.value)} className="field"/></Field><Field label="Unit cost (minor) "><input required type="number" min="0" step="1" value={line.unit_cost_minor} onChange={e => updateLine(i,"unit_cost_minor",e.target.value)} className="field"/></Field><button type="button" disabled={draftLines.length === 1} onClick={() => setDraftLines(old => old.filter((_, index) => index !== i))} className="mt-6 rounded-lg p-2 text-stone-400 hover:bg-red-50 hover:text-red-700 disabled:opacity-30"><X size={17}/></button></div>)}</div><Field label="Notes"><textarea value={notes} onChange={e => setNotes(e.target.value)} rows={2} className="field mt-1" placeholder="Optional supplier or delivery notes"/></Field><div className="mt-6 flex justify-end gap-2"><button type="button" onClick={() => setShowCreate(false)} className="rounded-lg border px-4 py-2.5 text-sm">Close</button><button disabled={saving || !suppliers.length || !products.length} className="rounded-lg bg-stone-900 px-5 py-2.5 text-sm font-medium text-white disabled:opacity-50">{saving ? "Saving…" : "Create draft"}</button></div></form></div>}

    {detail && <div className="fixed inset-0 z-50 flex justify-end bg-black/40"><section className="h-full w-full max-w-2xl overflow-y-auto bg-white p-6 shadow-2xl"><div className="flex items-start justify-between"><div><button onClick={() => setDetail(null)} className="mb-4 inline-flex items-center gap-1 text-sm text-stone-500 hover:text-stone-900"><ArrowLeft size={15}/> Close details</button><p className="text-xs font-semibold uppercase tracking-widest text-amber-700">Purchase order</p><h2 className="mt-1 text-2xl font-semibold">{detail.order.po_number}</h2><p className="mt-1 text-sm text-stone-500">{detail.order.supplier_name}</p></div><Status value={detail.order.status}/></div><div className="mt-6 overflow-hidden rounded-xl border border-stone-200"><div className="border-b bg-stone-50 px-4 py-3 font-semibold">Ordered products</div><div className="divide-y divide-stone-100">{detail.lines.map(l => <div key={l.id} className="p-4"><div className="flex justify-between gap-3"><div><p className="font-medium">{l.product_name}</p><p className="mt-1 text-xs text-stone-500">Unit cost {money(l.unit_cost_minor)}</p></div><p className="text-right text-sm"><span className="font-semibold">{l.received_quantity}</span><span className="text-stone-400"> / {l.ordered_quantity}</span><span className="block text-xs text-stone-500">received / ordered</span></p></div>{(detail.order.status === "ordered" || detail.order.status === "partially_received") && <div className="mt-3 flex items-center justify-between"><label className="text-xs text-stone-500">Receive now</label><input aria-label={`Receive quantity for ${l.product_name}`} type="number" min="0" max={Number(l.remaining_quantity)} step="0.0001" value={receiveQty[l.id] ?? "0"} onChange={e => setReceiveQty(old => ({ ...old, [l.id]: e.target.value }))} className="w-36 rounded-lg border border-stone-300 px-3 py-2 text-right text-sm"/></div>}<p className="mt-2 text-xs text-stone-400">Remaining: {l.remaining_quantity}</p></div>)}</div></div>
      {(detail.order.status === "ordered" || detail.order.status === "partially_received") && <div className="mt-5 rounded-xl border border-amber-200 bg-amber-50 p-4"><div className="flex items-center gap-2 font-semibold"><Truck size={17}/> Receive delivery</div><p className="mt-1 text-xs text-stone-600">Only enter quantities physically received. Inventory is updated by the receiving API.</p><input value={receiptNote} onChange={e => setReceiptNote(e.target.value)} placeholder="Delivery note (optional)" className="mt-3 w-full rounded-lg border border-amber-200 bg-white px-3 py-2 text-sm"/><button disabled={saving} onClick={() => void receive()} className="mt-3 w-full rounded-lg bg-amber-700 px-4 py-2.5 text-sm font-semibold text-white hover:bg-amber-800 disabled:opacity-50">{saving ? "Processing…" : "Record goods received"}</button></div>}
      <div className="mt-6"><h3 className="font-semibold">Receipt history</h3>{detail.receipts.length ? <div className="mt-3 space-y-2">{detail.receipts.map(r => <div key={r.id} className="rounded-lg border border-stone-200 p-3"><p className="text-sm font-medium">Receipt recorded</p><p className="mt-1 text-xs text-stone-500">{date(r.created_at)} · {r.note || "No note"}</p>{r.lines?.length > 0 && <div className="mt-3 space-y-2 border-t border-stone-100 pt-2">{r.lines.map(line => <div key={line.id} className="flex items-center justify-between gap-3 text-xs"><span className="text-stone-600">{line.product_name}</span><span className="font-medium tabular-nums">{line.received_quantity} received</span></div>)}</div>}</div>)}</div> : <p className="mt-2 text-sm text-stone-500">No deliveries recorded yet.</p>}</div>
      <div className="mt-8 flex flex-wrap gap-2 border-t border-stone-100 pt-5">{detail.order.status === "draft" && <button disabled={saving} onClick={() => void transition("order")} className="rounded-lg bg-stone-900 px-4 py-2.5 text-sm font-medium text-white">Mark as ordered</button>}{(detail.order.status === "draft" || detail.order.status === "ordered") && <button disabled={saving} onClick={() => void transition("cancel")} className="rounded-lg border border-red-200 px-4 py-2.5 text-sm font-medium text-red-700 hover:bg-red-50">Cancel order</button>}</div>
    </section></div>}
  </main>;
}
function Stat({ label, value }: { label: string; value: number }) { return <div className="rounded-xl border border-stone-200 bg-white p-5"><p className="text-sm text-stone-500">{label}</p><p className="mt-2 text-3xl font-semibold tabular-nums">{value}</p></div>; }
function Status({ value }: { value: string }) { const tone = value === "received" ? "bg-emerald-50 text-emerald-700" : value === "cancelled" ? "bg-red-50 text-red-700" : value === "partially_received" ? "bg-amber-50 text-amber-800" : value === "ordered" ? "bg-blue-50 text-blue-700" : "bg-stone-100 text-stone-600"; return <span className={`inline-flex rounded-full px-2.5 py-1 text-xs font-semibold capitalize ${tone}`}>{value.replaceAll("_", " ")}</span>; }
function Field({ label, children }: { label: string; children: ReactNode }) { return <label className="block min-w-0 text-xs font-medium text-stone-600">{label}{children}</label>; }
