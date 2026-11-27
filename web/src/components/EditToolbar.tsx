import type { LayoutDevice } from '../types'
import { EyeIcon, PlusIcon, RedoIcon, SaveIcon, UndoIcon } from './Icons'

export function EditToolbar({
  device,
  onDeviceChange,
  dirty,
  preview,
  canUndo,
  canRedo,
  saving,
  onAdd,
  onUndo,
  onRedo,
  onPreview,
  onCancel,
  onSave,
}: {
  device: LayoutDevice
  onDeviceChange: (device: LayoutDevice) => void
  dirty: boolean
  preview: boolean
  canUndo: boolean
  canRedo: boolean
  saving: boolean
  onAdd: () => void
  onUndo: () => void
  onRedo: () => void
  onPreview: () => void
  onCancel: () => void
  onSave: () => void
}) {
  return (
    <div className={`edit-toolbar${preview ? ' is-preview' : ''}`} role="toolbar" aria-label="首页编辑工具栏">
      <div className="edit-toolbar-main">
        {!preview && (
          <button type="button" className="primary-button edit-add" onClick={onAdd}>
            <PlusIcon size={16} />
            新增
          </button>
        )}
        <button type="button" className="toolbar-button" onClick={onUndo} disabled={!canUndo || saving} title="撤销最近一次编辑">
          <UndoIcon size={16} />
          撤销
        </button>
        <button type="button" className="toolbar-button" onClick={onRedo} disabled={!canRedo || saving} title="重做已撤销的编辑">
          <RedoIcon size={16} />
          重做
        </button>
        <div className="device-switch" role="group" aria-label="编辑布局设备">
          <button
            type="button"
            className={device === 'desktop' ? 'is-active' : ''}
            aria-pressed={device === 'desktop'}
            onClick={() => onDeviceChange('desktop')}
          >
            桌面
          </button>
          <button
            type="button"
            className={device === 'mobile' ? 'is-active' : ''}
            aria-pressed={device === 'mobile'}
            onClick={() => onDeviceChange('mobile')}
          >
            移动
          </button>
        </div>
        <span className={`edit-state${dirty ? ' is-dirty' : ''}`} aria-live="polite">
          {preview ? '预览中 · 未写入正式数据' : dirty ? '有未保存修改' : '当前为已保存布局'}
        </span>
      </div>
      <div className="edit-toolbar-actions">
        <button type="button" className="toolbar-button" onClick={onPreview} disabled={saving}>
          <EyeIcon size={16} />
          {preview ? '返回编辑' : '预览'}
        </button>
        <button type="button" className="toolbar-button" onClick={onCancel} disabled={saving}>
          取消
        </button>
        <button type="button" className="primary-button toolbar-save" onClick={onSave} disabled={saving}>
          <SaveIcon size={16} />
          {saving ? '保存中…' : '保存'}
        </button>
      </div>
    </div>
  )
}
