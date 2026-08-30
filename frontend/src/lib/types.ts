export type Article = { id:number; teacher_id?:number; title:string; content:string; created_at:string; uploader?:string; mine?:number; picked?:number }
export type Competition = { id:number; teacher_id:number; article_id:number; access_code:string; status:'waiting'|'running'|'stopped'; time_limit:number; pinyin_enabled:boolean; started_at?:string; created_at:string; article_title:string }
export type Result = { id:number; competition_id:number; student_id:string; student_name:string; class_id?:number|null; class_name?:string; correct_count:number; total_count:number; accuracy:number; speed:number; duration:number; created_at:string; access_code:string; article_title:string }
export type ClassItem = { id:number; name:string; student_count:number; created_at:string }
export type Student = { student_id?:string; student_name?:string; studentId?:string; studentName?:string }
export type Teacher = { id:number; username:string; display_name:string; is_admin:number; created_at:string }
