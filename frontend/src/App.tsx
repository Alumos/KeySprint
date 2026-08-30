import { lazy, Suspense } from 'react'
import { Navigate, Route, Routes } from 'react-router-dom'
const Setup=lazy(()=>import('./pages/Setup'))
const TeacherLogin=lazy(()=>import('./pages/TeacherLogin'))
const Admin=lazy(()=>import('./pages/Admin'))
const StudentLogin=lazy(()=>import('./pages/StudentLogin'))
const Compete=lazy(()=>import('./pages/Compete'))

export default function App(){return <div className="app-root"><div className="app-content"><Suspense fallback={<div className="race-loading">正在加载...</div>}><Routes>
  <Route path="/" element={<Navigate to="/student" replace/>}/>
  <Route path="/setup" element={<Setup/>}/>
  <Route path="/admin" element={<TeacherLogin/>}/>
  <Route path="/admin/dashboard" element={<Admin/>}/>
  <Route path="/student" element={<StudentLogin/>}/>
  <Route path="/student/compete" element={<Compete/>}/>
  <Route path="*" element={<Navigate to="/student" replace/>}/>
</Routes></Suspense></div><footer className="site-footer"><a href="https://beian.miit.gov.cn/" target="_blank" rel="noopener noreferrer nofollow">苏ICP备2021038338号-1</a></footer></div>}
