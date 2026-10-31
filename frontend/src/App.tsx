import { lazy, Suspense } from 'react'
import { Navigate, Route, Routes } from 'react-router-dom'
import { SiteProvider, useSite } from './lib/site'
const Setup=lazy(()=>import('./pages/Setup'))
const TeacherLogin=lazy(()=>import('./pages/TeacherLogin'))
const Admin=lazy(()=>import('./pages/Admin'))
const StudentLogin=lazy(()=>import('./pages/StudentLogin'))
const Compete=lazy(()=>import('./pages/Compete'))

// The filing number is operator supplied; when it is empty the footer simply
// shows nothing instead of a hard-coded number.
function SiteFooter(){const{site}=useSite();return <footer className="site-footer">{site.icp?<a href="https://beian.miit.gov.cn/" target="_blank" rel="noopener noreferrer nofollow">{site.icp}</a>:null}</footer>}

export default function App(){return <SiteProvider><div className="app-root"><div className="app-content"><Suspense fallback={<div className="race-loading">正在加载...</div>}><Routes>
  <Route path="/" element={<Navigate to="/student" replace/>}/>
  <Route path="/setup" element={<Setup/>}/>
  <Route path="/admin" element={<TeacherLogin/>}/>
  <Route path="/admin/dashboard" element={<Admin/>}/>
  <Route path="/student" element={<StudentLogin/>}/>
  <Route path="/student/compete" element={<Compete/>}/>
  <Route path="*" element={<Navigate to="/student" replace/>}/>
</Routes></Suspense></div><SiteFooter/></div></SiteProvider>}
