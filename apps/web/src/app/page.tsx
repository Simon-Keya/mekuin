import Link from "next/link";
const modules = [
  ["Point of Sale", "Fast service across walk-in, table and tab orders", "01"],
  ["Stock control", "Inventory, recipes, counts and reconciliation", "02"],
  ["Procurement", "Suppliers, purchase orders and receiving", "03"],
  ["Cash & insight", "Shifts, expenses, reports and audit trails", "04"],
];
export default function Home() {
  return <main className="min-h-screen">
    <header className="flex items-center justify-between border-b border-[#e9e5df] bg-white px-8 py-5">
      <div><div className="text-xl font-black tracking-tight">MEKU<span className="text-[#d9912e]">-In</span></div><div className="text-[10px] tracking-[.22em] text-stone-500">BY MEKU SOLUTIONS</div></div>
      <Link href="/login" className="rounded-md bg-[#171717] px-5 py-2.5 text-sm font-semibold text-white">Sign in</Link>
    </header>
    <section className="mx-auto grid max-w-7xl gap-12 px-8 py-24 lg:grid-cols-[1.1fr_.9fr]">
      <div className="self-center">
        <p className="mb-5 text-xs font-bold uppercase tracking-[.25em] text-[#b77a22]">Operations platform for hospitality</p>
        <h1 className="max-w-3xl text-5xl font-semibold leading-[1.06] tracking-tight md:text-7xl">Know what you sell. Know what you stock. Know your numbers.</h1>
        <p className="mt-7 max-w-xl text-lg leading-8 text-stone-600">MEKU-In brings point of sale, recipe-based stock control, procurement and cash operations into one workspace for bars, lounges and nightclubs.</p>
        <div className="mt-9 flex gap-3"><Link href="/login" className="rounded-md bg-[#e9a23b] px-6 py-3 font-semibold text-[#171717]">Open workspace</Link><a href="#platform" className="rounded-md border border-stone-300 bg-white px-6 py-3 font-semibold">Explore platform</a></div>
      </div>
      <div className="rounded-2xl border border-stone-200 bg-white p-5 shadow-xl shadow-stone-200/50">
        <div className="flex items-center justify-between border-b border-stone-100 pb-4"><span className="font-semibold">Operations overview</span><span className="rounded-full bg-amber-50 px-3 py-1 text-xs text-amber-800">Preview</span></div>
        <div className="grid grid-cols-2 gap-3 py-5">{[["Today's sales","KES 184,650"],["Orders","112"],["Low stock","07"],["Open shift","02"]].map(([a,b])=><div key={a} className="rounded-xl bg-stone-50 p-4"><p className="text-xs text-stone-500">{a}</p><p className="mt-2 text-2xl font-semibold">{b}</p></div>)}</div>
        <div className="rounded-xl bg-[#171717] p-5 text-white"><p className="text-xs text-stone-400">CONTROL YOUR OPERATION</p><div className="mt-3 h-2 overflow-hidden rounded bg-stone-700"><div className="h-full w-[72%] rounded bg-[#e9a23b]"/></div><div className="mt-2 flex justify-between text-xs text-stone-400"><span>Stock visibility</span><span>72%</span></div></div>
        <p className="mt-3 text-xs text-stone-400">Illustrative product preview — not live business data.</p>
      </div>
    </section>
    <section id="platform" className="border-y border-stone-200 bg-white px-8 py-16"><div className="mx-auto max-w-7xl"><p className="text-xs font-bold uppercase tracking-[.2em] text-stone-500">One connected operation</p><div className="mt-8 grid gap-4 md:grid-cols-4">{modules.map(([name,desc,n])=><article key={n} className="border-t-2 border-[#e9a23b] pt-5"><p className="text-xs text-stone-400">{n}</p><h2 className="mt-4 text-lg font-semibold">{name}</h2><p className="mt-2 text-sm leading-6 text-stone-500">{desc}</p></article>)}</div></div></section>
    <footer className="px-8 py-8 text-center text-xs text-stone-500">MEKU-In · A MEKU SOLUTIONS product</footer>
  </main>;
}
