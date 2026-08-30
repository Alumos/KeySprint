import { Keyboard, LockKeyhole } from 'lucide-react'
import { FormEvent, useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { Button, Field, Input } from '../components/ui'
import { api, json } from '../lib/api'

export default function TeacherLogin(){const nav=useNavigate();const[username,setUsername]=useState('');const[password,setPassword]=useState('');const[error,setError]=useState('');const[busy,setBusy]=useState(false)
  useEffect(()=>{api<{done:boolean}>('/api/setup/status').then(x=>{if(!x.done)nav('/setup',{replace:true})}).catch(()=>{})},[nav])
  async function submit(e:FormEvent){e.preventDefault();setBusy(true);setError('');try{const x=await api<any>('/api/admin/login',json('POST',{username,password}));sessionStorage.setItem('adminToken',x.token);sessionStorage.setItem('displayName',x.displayName);sessionStorage.setItem('canManageTeachers',String(x.isAdmin));nav('/admin/dashboard')}catch(e){setError((e as Error).message)}finally{setBusy(false)}}
  return <main className="auth-shell"><section className="auth-card"><div className="auth-brand"><div className="brand-mark"><LockKeyhole size={18}/></div><h1>教师控制台</h1><p>使用教师账号登录 KeySprint</p></div><form className="auth-form" onSubmit={submit}><Field label="用户名"><Input value={username} onChange={e=>setUsername(e.target.value)} autoFocus autoComplete="username"/></Field><Field label="密码"><Input value={password} onChange={e=>setPassword(e.target.value)} type="password" autoComplete="current-password"/></Field>{error&&<div className="notice notice-error">{error}</div>}<Button disabled={busy||!username||!password}>{busy?'正在登录...':'登录'}</Button></form><div className="auth-links"><Link to="/student"><Button variant="ghost" size="sm"><Keyboard size={14}/>学生入口</Button></Link></div></section></main>}
