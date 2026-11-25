"use client";
import Link from "next/link";
import { FormEvent, useEffect, useState } from "react";
const API = process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080";
export default function AcceptInvitationPage() {
  const [token,setToken]=useState("");
  const [authenticated,setAuthenticated]=useState<boolean|null>(null);
  const [busy,setBusy]=useState(false);
  const [error,setError]=useState("");
  const [accepted,setAccepted]=useState(false);
  const [fullName,setFullName]=useState("");
  const [password,setPassword]=useState("");
  useEffect(()=>{
    const value=new URLSearchParams(window.location.search).get("token")??"";
    setToken(value);
    if(!/^[a-fA-F0-9]{64}$/.test(value)){setError("This invitation link is invalid or incomplete.");setAuthenticated(false);return;}
    fetch(`${API}/api/v1/auth/me`,{credentials:"include"}).then(r=>setAuthenticated(r.ok)).catch(()=>setAuthenticated(false));
  },[]);
  async function accept(){setBusy(true);setError("");try{
    const response=await fetch(`${API}/api/v1/auth/invitations/accept`,{method:"POST",credentials:"include",headers:{"Content-Type":"application/json"},body:JSON.stringify({token})});
    const data=await response.json();if(!response.ok)throw new Error(data.error??"Invitation could not be accepted");setAccepted(true);
  }catch(e){setError(e instanceof Error?e.message:"Invitation acceptance failed");}finally{setBusy(false);}}
  async function registerAndAccept(e:FormEvent){e.preventDefault();setBusy(true);setError("");try{
    const registration=await fetch(`${API}/api/v1/auth/invitations/register`,{method:"POST",credentials:"include",headers:{"Content-Type":"application/json"},body:JSON.stringify({token,full_name:fullName,password})});
    const registered=await registration.json();if(!registration.ok)throw new Error(registered.error??"Account registration failed");setAuthenticated(true);
    const acceptance=await fetch(`${API}/api/v1/auth/invitations/accept`,{method:"POST",credentials:"include",headers:{"Content-Type":"application/json"},body:JSON.stringify({token})});
    const acceptedData=await acceptance.json();if(!acceptance.ok)throw new Error(acceptedData.error??"Invitation could not be accepted");setAccepted(true);
  }catch(e){setError(e instanceof Error?e.message:"Invitation registration failed");}finally{setBusy(false);}}
  const loginHref=`/login?next=${encodeURIComponent(`/accept-invitation?token=${encodeURIComponent(token)}`)}`;
  return <main className="grid min-h-screen place-items-center px-5"><section className="w-full max-w-lg rounded-2xl border border-stone-200 bg-white p-8 shadow-sm">
    <Link href="/" className="text-xl font-black">MEKU<span className="text-[#d9912e]">-In</span></Link><p className="mt-1 text-xs tracking-[.18em] text-stone-500">BY MEKU SOLUTIONS</p>
    {accepted?<><h1 className="mt-10 text-3xl font-semibold">You’re in.</h1><p className="mt-3 text-sm text-stone-600">Your staff membership is active. Open your workspace to choose the business and branch you’ve been assigned.</p><Link href="/workspace" className="mt-8 inline-flex rounded-lg bg-stone-900 px-5 py-3 font-semibold text-white">Continue to workspace</Link></>:<>
      <h1 className="mt-10 text-3xl font-semibold">Accept your invitation</h1><p className="mt-3 text-sm text-stone-600">Sign in with the email address that received this invitation. The invitation can only be used once and expires after 72 hours.</p>
      {authenticated===null&&<p className="mt-6 text-sm text-stone-500">Checking your sign-in…</p>}
      {authenticated===false&&token&&<div className="mt-7"><p className="text-sm text-stone-600">New to MEKU-In? Create your account using the invited email address. Your account email is taken from the invitation and verified through this link.</p><form onSubmit={registerAndAccept} className="mt-5 space-y-4"><label className="block text-sm font-medium">Your full name<input required minLength={2} maxLength={120} value={fullName} onChange={e=>setFullName(e.target.value)} className="mt-2 w-full rounded-lg border border-stone-300 px-3 py-3"/></label><label className="block text-sm font-medium">Create a password<input required minLength={12} maxLength={72} type="password" value={password} onChange={e=>setPassword(e.target.value)} className="mt-2 w-full rounded-lg border border-stone-300 px-3 py-3"/></label><button disabled={busy} className="w-full rounded-lg bg-stone-900 px-5 py-3 font-semibold text-white disabled:opacity-50">{busy?"Creating account…":"Create account and accept"}</button></form><p className="mt-5 text-center text-sm text-stone-500">Already have an account? <Link href={loginHref} className="font-semibold underline">Sign in instead</Link></p></div>}
      {authenticated===true&&<button disabled={busy||!token} onClick={accept} className="mt-8 w-full rounded-lg bg-stone-900 px-5 py-3 font-semibold text-white disabled:opacity-50">{busy?"Accepting invitation…":"Accept invitation"}</button>}
      {error&&<p role="alert" className="mt-5 text-sm text-red-700">{error.replaceAll("_"," ")}</p>}
      <p className="mt-8 text-xs text-stone-500">If you were not expecting this invitation, you can close this page. Never forward your invitation link.</p>
    </>}
  </section></main>;
}
