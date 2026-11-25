"use client";
import Link from "next/link";
import { FormEvent, useState } from "react";
import GoogleSignIn from "./google-sign-in";
const API = process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080";
export default function LoginPage() {
  const [email,setEmail]=useState(""); const [password,setPassword]=useState("");
  const [error,setError]=useState(""); const [busy,setBusy]=useState(false); const [mfa,setMfa]=useState(""); const [mfaCode,setMfaCode]=useState(""); const [challenge,setChallenge]=useState("");
  async function submit(e:FormEvent){e.preventDefault();setBusy(true);setError("");
    try { const res=await fetch(`${API}/api/v1/auth/login`,{method:"POST",credentials:"include",headers:{"Content-Type":"application/json"},body:JSON.stringify({email,password})});
      const data=await res.json(); if(!res.ok) throw new Error(data.error??"Sign in failed"); if(data.status==="mfa_required"){setChallenge(data.challenge_id);setMfa("pending");return;}
      const next=new URLSearchParams(window.location.search).get("next");
      window.location.href=next && next.startsWith("/") && !next.startsWith("//") ? next : "/workspace";
    } catch(err){setError(err instanceof Error?err.message:"Unable to sign in");} finally{setBusy(false);}
  }
  async function verifyMFA(e:FormEvent){e.preventDefault();setBusy(true);setError("");try{const res=await fetch(`${API}/api/v1/auth/mfa/verify`,{method:"POST",credentials:"include",headers:{"Content-Type":"application/json"},body:JSON.stringify({challenge_id:challenge,code:mfaCode})});const data=await res.json();if(!res.ok)throw new Error(data.error??"MFA verification failed");window.location.href="/workspace";}catch(err){setError(err instanceof Error?err.message:"MFA verification failed");}finally{setBusy(false);}}
  return <main className="grid min-h-screen place-items-center px-5"><section className="w-full max-w-md rounded-2xl border border-stone-200 bg-white p-8 shadow-sm">
    <Link href="/" className="text-xl font-black">MEKU<span className="text-[#d9912e]">-In</span></Link><p className="mt-1 text-xs tracking-[.18em] text-stone-500">BY MEKU SOLUTIONS</p>
    <h1 className="mt-10 text-3xl font-semibold">{mfa==="pending"?"Two-factor verification":"Welcome back"}</h1><p className="mt-2 text-sm text-stone-500">{mfa==="pending"?"Enter the current six-digit code from your authenticator app.":"Sign in to your business workspace."}</p>
    {mfa==="pending"?<form className="mt-8 space-y-5" onSubmit={verifyMFA}><label className="block text-sm font-medium">Authenticator code<input required pattern="[0-9]{6}" maxLength={6} inputMode="numeric" value={mfaCode} onChange={e=>setMfaCode(e.target.value)} className="mt-2 w-full rounded-lg border border-stone-300 px-3 py-3"/></label><button disabled={busy} className="w-full rounded-lg bg-[#171717] py-3 font-semibold text-white">{busy?"Verifying…":"Verify and sign in"}</button>{error&&<p role="alert" className="text-sm text-red-700">{error.replaceAll("_"," ")}</p>}</form>:<><form className="mt-8 space-y-5" onSubmit={submit}>
      <label className="block text-sm font-medium">Email<input required type="email" autoComplete="email" value={email} onChange={e=>setEmail(e.target.value)} className="mt-2 w-full rounded-lg border border-stone-300 px-3 py-3 outline-none focus:border-amber-500"/></label>
      <label className="block text-sm font-medium">Password<input required type="password" autoComplete="current-password" value={password} onChange={e=>setPassword(e.target.value)} className="mt-2 w-full rounded-lg border border-stone-300 px-3 py-3 outline-none focus:border-amber-500"/></label>
      {error&&<p role="alert" className="text-sm text-red-700">{error.replaceAll("_"," ")} {error.includes("email_verification_required")&&<Link href="/verify-email" className="underline">Verify email</Link>}</p>}
      <button disabled={busy} className="w-full rounded-lg bg-[#171717] py-3 font-semibold text-white disabled:opacity-60">{busy?"Signing in…":"Sign in"}</button>
    </form><div className="text-center text-sm"><Link href="/forgot-password" className="underline">Forgot password?</Link></div><GoogleSignIn/><p className="mt-6 text-center text-sm text-stone-500">New to MEKU-In? <Link className="font-semibold text-stone-900 underline" href="/register">Create a workspace</Link></p></>}
  </section></main>;
}
