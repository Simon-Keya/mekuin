"use client";
import Link from "next/link";
import { FormEvent,useState } from "react";
const API=process.env.NEXT_PUBLIC_API_BASE_URL??"http://localhost:8080";
export default function RegisterPage(){
 const [form,setForm]=useState({full_name:"",email:"",password:"",business_name:"",branch_name:"Main Branch",phone:""});
 const [error,setError]=useState("");const [busy,setBusy]=useState(false);
 function update(k:keyof typeof form,v:string){setForm(p=>({...p,[k]:v}));}
 async function submit(e:FormEvent){e.preventDefault();setBusy(true);setError("");try{const res=await fetch(`${API}/api/v1/auth/register`,{method:"POST",credentials:"include",headers:{"Content-Type":"application/json"},body:JSON.stringify(form)});const data=await res.json();if(!res.ok)throw new Error(data.error??"Could not create workspace");window.location.href=`/verify-email?email=${encodeURIComponent(form.email)}`;}catch(err){setError(err instanceof Error?err.message:"Unable to register");}finally{setBusy(false);}}

 const fields:[keyof typeof form,string,string][]=[["full_name","Your full name","text"],["email","Work email","email"],["business_name","Business / venue name","text"],["branch_name","First branch name","text"],["phone","Business phone (optional)","tel"],["password","Password (minimum 12 characters)","password"]];
 return <main className="grid min-h-screen place-items-center px-5 py-10"><section className="w-full max-w-lg rounded-2xl border border-stone-200 bg-white p-8 shadow-sm"><Link href="/" className="text-xl font-black">MEKU<span className="text-[#d9912e]">-In</span></Link><h1 className="mt-8 text-3xl font-semibold">Create your workspace</h1><p className="mt-2 text-sm text-stone-500">Your account becomes the owner of the first business workspace. Email verification is required before sign-in.</p>
 <form className="mt-7 grid gap-4 sm:grid-cols-2" onSubmit={submit}>{fields.map(([key,label,type])=><label key={key} className={`block text-sm font-medium ${key==="password"||key==="business_name"?"sm:col-span-2":""}`}>{label}<input required={key!=="phone"} minLength={key==="password"?12:undefined} type={type} autoComplete={key==="password"?"new-password":key==="email"?"email":"off"} value={form[key]} onChange={e=>update(key,e.target.value)} className="mt-2 w-full rounded-lg border border-stone-300 px-3 py-3"/></label>)}<button disabled={busy} className="sm:col-span-2 w-full rounded-lg bg-[#171717] py-3 font-semibold text-white">{busy?"Creating workspace…":"Create workspace"}</button></form>
 {error&&<p role="alert" className="mt-4 text-sm text-red-700">{error.replaceAll("_"," ")} {error.includes("account_created_email_delivery_unavailable")&&<Link href={`/verify-email?email=${encodeURIComponent(form.email)}`} className="underline">Continue to email verification</Link>}</p>}<p className="mt-5 text-center text-sm text-stone-500">Already registered? <Link href="/login" className="font-semibold underline">Sign in</Link></p></section></main>;
}
