"use client";
import { useEffect,useState } from "react";
type Branch={id:string;name:string}; type Business={id:string;name:string;role:string;branches:Branch[]};
type Me={id:string;full_name:string;email:string};
const API=process.env.NEXT_PUBLIC_API_BASE_URL??"http://localhost:8080";
export default function WorkspacePage(){
 const [me,setMe]=useState<Me|null>(null);const [businesses,setBusinesses]=useState<Business[]>([]);const [error,setError]=useState("");const [selecting,setSelecting]=useState("");
 useEffect(()=>{(async()=>{try{
  const [u,b]=await Promise.all([fetch(`${API}/api/v1/auth/me`,{credentials:"include"}),fetch(`${API}/api/v1/workspace`,{credentials:"include"})]);
  if(u.status===401||b.status===401){window.location.href="/login";return;}
  if(!u.ok||!b.ok)throw new Error("Workspace could not be loaded");
  setMe(await u.json());const data=await b.json();setBusinesses(data.businesses??[]);
 }catch(e){setError(e instanceof Error?e.message:"Unable to load workspace");}})();},[]);
 async function selectWorkspace(businessId:string,branchId:string){setSelecting(branchId);setError("");try{const res=await fetch(`${API}/api/v1/tenant/select`,{method:"POST",credentials:"include",headers:{"Content-Type":"application/json"},body:JSON.stringify({business_id:businessId,branch_id:branchId})});if(!res.ok)throw new Error("Could not select this workspace. Please refresh and try again.");window.location.href="/app";}catch(e){setError(e instanceof Error?e.message:"Workspace selection failed");setSelecting("");}}
 async function logout(){await fetch(`${API}/api/v1/auth/logout`,{method:"POST",credentials:"include"});window.location.href="/";}
 return <main className="min-h-screen"><header className="flex items-center justify-between border-b bg-white px-8 py-5"><div className="text-xl font-black">MEKU<span className="text-[#d9912e]">-In</span></div><button onClick={logout} className="text-sm underline">Sign out</button></header><section className="mx-auto max-w-5xl px-6 py-14">
  <p className="text-xs font-bold uppercase tracking-[.2em] text-amber-700">Your workspace</p><h1 className="mt-3 text-4xl font-semibold">Good to see you{me?`, ${me.full_name.split(" ")[0]}`:""}.</h1><p className="mt-2 text-stone-500">Select a business and branch to continue.</p>
  {error&&<p role="alert" className="mt-8 text-red-700">{error}</p>}
  {!error&&businesses.length===0&&<div className="mt-8 rounded-xl border bg-white p-6"><p className="font-semibold">No active workspace is assigned.</p><p className="mt-2 text-sm text-stone-500">Ask your business owner to invite you, or create a new workspace.</p></div>}
  <div className="mt-8 grid gap-4 md:grid-cols-2">{businesses.map(b=><article key={b.id} className="rounded-xl border border-stone-200 bg-white p-6"><div className="flex justify-between"><h2 className="text-xl font-semibold">{b.name}</h2><span className="rounded-full bg-amber-50 px-3 py-1 text-xs capitalize text-amber-900">{b.role.replaceAll("_"," ")}</span></div><p className="mt-5 text-xs uppercase tracking-wider text-stone-400">Available branches</p><ul className="mt-2 space-y-2">{b.branches.map(br=><li key={br.id} className="flex items-center justify-between gap-3 rounded-lg bg-stone-50 px-4 py-3 text-sm"><span>{br.name}</span><button disabled={!!selecting} onClick={()=>selectWorkspace(b.id,br.id)} className="rounded-lg bg-stone-900 px-3 py-2 text-xs font-semibold text-white disabled:opacity-50">{selecting===br.id?"Selecting…":"Open workspace"}</button></li>)}</ul></article>)}</div>
 </section></main>;
}
