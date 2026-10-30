import * as Dialog from '@radix-ui/react-dialog'
import * as DropdownMenu from '@radix-ui/react-dropdown-menu'
import * as Select from '@radix-ui/react-select'
import { Check, ChevronDown, X } from 'lucide-react'
import type { ButtonHTMLAttributes, InputHTMLAttributes, ReactNode } from 'react'
import { TEXT_OPTION_ITEMS, type TextOptions } from '../lib/textOptions'

export function Button({variant='default',size='default',className='',...props}:ButtonHTMLAttributes<HTMLButtonElement>&{variant?:'default'|'outline'|'ghost'|'danger'|'secondary';size?:'default'|'sm'|'icon'}){
  return <button className={`button button-${variant} button-${size} ${className}`} {...props}/>
}
export function Input(props:InputHTMLAttributes<HTMLInputElement>){return <input {...props} className={`input ${props.className||''}`}/>}
export function Field({label,hint,children}: {label:string;hint?:string;children:ReactNode}){return <label className="field"><span className="field-label">{label}</span>{children}{hint&&<span className="field-hint">{hint}</span>}</label>}
export function Panel({children,className=''}:{children:ReactNode;className?:string}){return <section className={`panel ${className}`}>{children}</section>}
export function PanelHeader({title,description,action}:{title:string;description?:string;action?:ReactNode}){return <div className="panel-header"><div><h2>{title}</h2>{description&&<p>{description}</p>}</div>{action}</div>}
export function Badge({children,tone='neutral'}:{children:ReactNode;tone?:'neutral'|'success'|'warning'|'danger'|'blue'}){return <span className={`badge badge-${tone}`}>{children}</span>}
export function Empty({title,description}:{title:string;description?:string}){return <div className="empty"><strong>{title}</strong>{description&&<span>{description}</span>}</div>}

export function Modal({open,onOpenChange,title,description,children,footer}:{open:boolean;onOpenChange:(v:boolean)=>void;title:string;description?:string;children:ReactNode;footer?:ReactNode}){
  return <Dialog.Root open={open} onOpenChange={onOpenChange}><Dialog.Portal><Dialog.Overlay className="dialog-overlay"/><Dialog.Content className="dialog-content"><div className="dialog-head"><div><Dialog.Title>{title}</Dialog.Title>{description&&<Dialog.Description>{description}</Dialog.Description>}</div><Dialog.Close asChild><Button variant="ghost" size="icon" aria-label="关闭"><X size={16}/></Button></Dialog.Close></div><div className="dialog-body">{children}</div>{footer&&<div className="dialog-footer">{footer}</div>}</Dialog.Content></Dialog.Portal></Dialog.Root>
}

export function AppSelect({value,onValueChange,placeholder='请选择',items,disabled}:{value?:string;onValueChange:(v:string)=>void;placeholder?:string;items:{value:string;label:string}[];disabled?:boolean}){
  return <Select.Root value={value} onValueChange={onValueChange} disabled={disabled}><Select.Trigger className="select-trigger"><Select.Value placeholder={placeholder}/><Select.Icon><ChevronDown size={15}/></Select.Icon></Select.Trigger><Select.Portal><Select.Content className="select-content" position="popper" sideOffset={4}><Select.Viewport>{items.map(x=><Select.Item className="select-item" value={x.value} key={x.value}><Select.ItemText>{x.label}</Select.ItemText><Select.ItemIndicator><Check size={14}/></Select.ItemIndicator></Select.Item>)}</Select.Viewport></Select.Content></Select.Portal></Select.Root>
}

export function StatStrip({items}:{items:{label:string;value:ReactNode;detail?:string}[]}){return <div className="stat-strip">{items.map(x=><div className="stat-cell" key={x.label}><span>{x.label}</span><strong>{x.value}</strong>{x.detail&&<small>{x.detail}</small>}</div>)}</div>}

export function OptionsMenu({value,onChange,label='文本选项'}:{value:TextOptions;onChange:(next:TextOptions)=>void;label?:string}){
  return <DropdownMenu.Root><DropdownMenu.Trigger asChild><Button variant="outline">{label}<ChevronDown size={14}/></Button></DropdownMenu.Trigger><DropdownMenu.Portal><DropdownMenu.Content className="dropdown-content options-menu" align="start" sideOffset={5}>{TEXT_OPTION_ITEMS.map(item=><DropdownMenu.CheckboxItem className="options-menu-item" key={item.key} checked={value[item.key]} onCheckedChange={checked=>onChange({...value,[item.key]:checked===true} as TextOptions)} onSelect={event=>event.preventDefault()}><span className="options-check">{value[item.key]&&<Check size={13}/>}</span><span className="options-copy"><strong>{item.label}</strong><small>{item.hint}</small></span></DropdownMenu.CheckboxItem>)}</DropdownMenu.Content></DropdownMenu.Portal></DropdownMenu.Root>
}
