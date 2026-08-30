import { BarChart3, BookOpen, ChevronDown, GraduationCap, Keyboard, LockKeyhole, LogOut, Menu, Settings, Trophy, UserCog } from 'lucide-react'
import * as DropdownMenu from '@radix-ui/react-dropdown-menu'
import { lazy, Suspense, useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button, Field, Input, Modal } from '../components/ui'
import { api, json } from '../lib/api'

// Keep the dashboard shell small. The heavier tables/chart view only loads when
// its section is selected, which improves the first admin-page render on slow VPS
// connections and avoids downloading the charting library for every teacher.
const ArticlesView = lazy(() => import('./admin/ArticlesView'))
const ClassesView = lazy(() => import('./admin/ClassesView'))
const CompetitionsView = lazy(() => import('./admin/CompetitionsView'))
const IntegrationView = lazy(() => import('./admin/IntegrationView'))
const ResultsView = lazy(() => import('./admin/ResultsView'))
const TeachersView = lazy(() => import('./admin/TeachersView'))

type Section='articles'|'classes'|'competitions'|'results'|'teachers'|'integration'
const items=[{id:'articles' as Section,label:'文章管理',icon:BookOpen},{id:'classes' as Section,label:'班级管理',icon:GraduationCap},{id:'competitions' as Section,label:'竞赛控制',icon:Trophy},{id:'results' as Section,label:'成绩分析',icon:BarChart3},{id:'teachers' as Section,label:'教师管理',icon:UserCog},{id:'integration' as Section,label:'积分系统配置',icon:Settings}]

export default function Admin(){
  const nav=useNavigate();const[section,setSection]=useState<Section>('articles');const[mobile,setMobile]=useState(false);const[session,setSession]=useState<any>(null);const[toast,setToast]=useState('');const[passwordOpen,setPasswordOpen]=useState(false);const[oldPassword,setOld]=useState('');const[newPassword,setNew]=useState('');const[error,setError]=useState('')
  useEffect(()=>{if(!sessionStorage.getItem('adminToken')){nav('/admin',{replace:true});return}api('/api/session').then(setSession).catch(()=>nav('/admin',{replace:true}))},[nav])
  function notify(s:string){setToast(s);setTimeout(()=>setToast(''),2800)}
  function logout(){sessionStorage.clear();nav('/admin')}
  async function changePassword(){setError('');try{await api('/api/teacher/password',json('PUT',{oldPassword,newPassword}));setPasswordOpen(false);setOld('');setNew('');notify('密码已更新')}catch(e){setError((e as Error).message)}}
  const visible=items.filter(x=>(x.id!=='teachers'||session?.isAdmin)&&(x.id!=='integration'||session?.isAdmin));const current=visible.find(x=>x.id===section)||visible[0]
  return <div className="admin-shell"><aside className={`sidebar ${mobile?'sidebar-open':''}`}><div className="sidebar-brand"><div><Keyboard size={17}/></div><span><strong>KeySprint</strong><small>键跃竞赛管理</small></span></div><nav>{visible.map(x=><button key={x.id} className={section===x.id?'active':''} onClick={()=>{setSection(x.id);setMobile(false)}}><x.icon size={16}/><span>{x.label}</span></button>)}</nav><div className="sidebar-foot"><span>登录教师</span><strong>{session?.displayName||'加载中...'}</strong></div></aside>{mobile&&<div className="sidebar-scrim" onClick={()=>setMobile(false)}/>}<div className="admin-main"><header className="admin-topbar"><div className="topbar-title"><Button variant="ghost" size="icon" className="mobile-menu" onClick={()=>setMobile(true)}><Menu size={17}/></Button><div><h1>{current.label}</h1><span>KeySprint 教师工作台</span></div></div><DropdownMenu.Root><DropdownMenu.Trigger asChild><Button variant="outline" size="sm">{session?.displayName||'教师'}<ChevronDown size={14}/></Button></DropdownMenu.Trigger><DropdownMenu.Portal><DropdownMenu.Content className="dropdown-content" align="end" sideOffset={5}><DropdownMenu.Item className="dropdown-item" onSelect={()=>setPasswordOpen(true)}><LockKeyhole size={14}/>修改密码</DropdownMenu.Item><DropdownMenu.Separator className="dropdown-separator"/><DropdownMenu.Item className="dropdown-item danger" onSelect={logout}><LogOut size={14}/>退出登录</DropdownMenu.Item></DropdownMenu.Content></DropdownMenu.Portal></DropdownMenu.Root></header><div className="admin-workspace"><Suspense fallback={<div className="race-loading">正在加载...</div>}>{section==='articles'&&<ArticlesView notify={notify}/>} {section==='classes'&&<ClassesView notify={notify}/>} {section==='competitions'&&<CompetitionsView notify={notify} openResults={id=>{sessionStorage.setItem('resultCompetition',String(id));setSection('results')}}/>} {section==='results'&&<ResultsView notify={notify}/>} {section==='teachers'&&session?.isAdmin&&<TeachersView notify={notify} currentId={session.id}/>} {section==='integration'&&session?.isAdmin&&<IntegrationView notify={notify}/>}</Suspense></div></div>{toast&&<div className="toast">{toast}</div>}<Modal open={passwordOpen} onOpenChange={setPasswordOpen} title="修改密码" description="修改后现有登录仍然有效" footer={<><Button variant="outline" onClick={()=>setPasswordOpen(false)}>取消</Button><Button onClick={changePassword} disabled={newPassword.length<6}>确认修改</Button></>}><div className="stack"><Field label="原密码"><Input type="password" value={oldPassword} onChange={e=>setOld(e.target.value)}/></Field><Field label="新密码" hint="至少 6 位"><Input type="password" value={newPassword} onChange={e=>setNew(e.target.value)}/></Field>{error&&<div className="notice notice-error">{error}</div>}</div></Modal></div>
}
