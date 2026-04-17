import React, { useState, useEffect, useRef } from 'react'
import { ModelMapping } from '../api/amp'
import { listAvailableModels, AvailableModel } from '../api/models'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Checkbox } from '@/components/ui/checkbox'
import { Textarea } from '@/components/ui/textarea'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

interface Props {
  mappings: ModelMapping[]
  onChange: (mappings: ModelMapping[]) => void
}

interface ChannelOption {
  channelId: string
  channelName: string
  channelType: AvailableModel['channelType']
}

const THINKING_LEVELS = [
  { value: '', label: '无' },
  { value: 'low', label: 'Low' },
  { value: 'medium', label: 'Medium' },
  { value: 'high', label: 'High' },
  { value: 'xhigh', label: 'XHigh' },
]

export default function ModelMappingEditor({ mappings, onChange }: Props) {
  const [availableModels, setAvailableModels] = useState<AvailableModel[]>([])
  const [loadingModels, setLoadingModels] = useState(false)
  const [showDropdown, setShowDropdown] = useState<number | null>(null)
  const [searchTerm, setSearchTerm] = useState('')
  const [dropdownPos, setDropdownPos] = useState({ top: 0, left: 0 })
  const dropdownRefs = useRef<(HTMLElement | null)[]>([])

  useEffect(() => {
    loadModels()
  }, [])

  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (showDropdown !== null) {
        const dropdown = document.getElementById('model-dropdown')
        const currentRef = dropdownRefs.current[showDropdown]
        if (dropdown && !dropdown.contains(event.target as Node) && 
            currentRef && !currentRef.contains(event.target as Node)) {
          setShowDropdown(null)
          setSearchTerm('')
        }
      }
    }
    const handleScroll = (event: Event) => {
      if (showDropdown !== null) {
        const dropdown = document.getElementById('model-dropdown')
        if (dropdown && dropdown.contains(event.target as Node)) {
          return
        }
        setShowDropdown(null)
        setSearchTerm('')
      }
    }
    document.addEventListener('mousedown', handleClickOutside)
    window.addEventListener('scroll', handleScroll, true)
    return () => {
      document.removeEventListener('mousedown', handleClickOutside)
      window.removeEventListener('scroll', handleScroll, true)
    }
  }, [showDropdown])

  const openDropdown = (index: number) => {
    const ref = dropdownRefs.current[index]
    if (ref) {
      const rect = ref.getBoundingClientRect()
      setDropdownPos({ top: rect.bottom, left: rect.left })
    }
    setShowDropdown(index)
  }

  const loadModels = async () => {
    try {
      setLoadingModels(true)
      const models = await listAvailableModels()
      setAvailableModels(models)
    } catch {
      // Silently fail - models will just not be available for selection
    } finally {
      setLoadingModels(false)
    }
  }

  const handleAdd = () => {
    onChange([...mappings, { from: '', to: '', channelId: '', regex: false, thinkingLevel: '', pseudoNonStream: false, auditKeywords: [], ampOnly: false, fastMode: false, customInstructions: '', customInstructionsEnabled: false }])
  }

  const handleRemove = (index: number) => {
    onChange(mappings.filter((_, i) => i !== index))
  }

  const handleChange = (index: number, field: keyof ModelMapping, value: string | boolean | string[]) => {
    const newMappings = [...mappings]
    newMappings[index] = { ...newMappings[index], [field]: value }
    onChange(newMappings)
  }

  const handleSelectModel = (index: number, model: AvailableModel) => {
    const newMappings = [...mappings]
    newMappings[index] = {
      ...newMappings[index],
      to: model.modelId,
      channelId: model.channelId,
    }
    onChange(newMappings)
    setShowDropdown(null)
    setSearchTerm('')
  }

  const filteredModels = searchTerm
    ? availableModels.filter(m => 
        m.modelId.toLowerCase().includes(searchTerm.toLowerCase()) ||
        m.displayName.toLowerCase().includes(searchTerm.toLowerCase())
      )
    : availableModels

  const groupedModels = filteredModels.reduce((acc, model) => {
    const key = model.channelType
    if (!acc[key]) acc[key] = []
    acc[key].push(model)
    return acc
  }, {} as Record<string, AvailableModel[]>)

  const availableChannels = availableModels.reduce((acc, model) => {
    if (!acc.some(channel => channel.channelId === model.channelId)) {
      acc.push({
        channelId: model.channelId,
        channelName: model.channelName,
        channelType: model.channelType,
      })
    }
    return acc
  }, [] as ChannelOption[])

  const renderToField = (index: number, mapping: ModelMapping, inputClassName = 'h-8') => (
    <div
      className="relative"
      ref={(el) => { dropdownRefs.current[index] = el }}
    >
      <div className="flex items-center gap-1">
        <Input
          value={mapping.to}
          onChange={(e) => handleChange(index, 'to', e.target.value)}
          onFocus={() => {
            if (availableModels.length > 0) {
              openDropdown(index)
            }
          }}
          placeholder="目标模型名"
          className={inputClassName}
        />
        {availableModels.length > 0 && (
          <Button
            type="button"
            variant="outline"
            size="icon"
            onClick={() => showDropdown === index ? setShowDropdown(null) : openDropdown(index)}
            className={inputClassName === 'h-8' ? 'h-8 w-8 shrink-0' : 'h-9 w-9 shrink-0'}
          >
            ▼
          </Button>
        )}
      </div>

      {showDropdown === index && (
        <div
          id="model-dropdown"
          className="fixed z-50 max-h-80 w-[min(24rem,calc(100vw-2rem))] overflow-auto rounded-md border bg-popover shadow-xl"
          style={{ top: dropdownPos.top, left: dropdownPos.left }}
        >
          <div className="sticky top-0 bg-popover p-2">
            <Input
              value={searchTerm}
              onChange={(e) => setSearchTerm(e.target.value)}
              placeholder="搜索模型..."
              className="h-8"
              autoFocus
            />
          </div>
          {loadingModels ? (
            <div className="p-3 text-center text-sm text-muted-foreground">加载中...</div>
          ) : Object.keys(groupedModels).length === 0 ? (
            <div className="p-3 text-center text-sm text-muted-foreground">无匹配模型</div>
          ) : (
            Object.entries(groupedModels).map(([type, models]) => (
              <div key={type}>
                <div className="bg-muted px-3 py-1 text-xs font-medium uppercase text-muted-foreground">
                  {type}
                </div>
                {models.map((model) => (
                  <button
                    key={`${model.channelId}-${model.modelId}`}
                    type="button"
                    onClick={() => handleSelectModel(index, model)}
                    className="block w-full px-3 py-2 text-left text-sm hover:bg-accent"
                  >
                    <div className="font-mono">{model.modelId}</div>
                    <div className="text-xs text-muted-foreground">{model.channelName}</div>
                  </button>
                ))}
              </div>
            ))
          )}
        </div>
      )}
    </div>
  )

  const renderChannelSelect = (index: number, mapping: ModelMapping, className = 'h-8') => (
    <select
      value={mapping.channelId || ''}
      onChange={(e) => handleChange(index, 'channelId', e.target.value)}
      className={`${className} w-full rounded-md border border-input bg-background px-2 text-sm ring-offset-background focus:outline-none focus:ring-2 focus:ring-ring`}
    >
      <option value="">自动选择</option>
      {mapping.channelId && !availableChannels.some(channel => channel.channelId === mapping.channelId) && (
        <option value={mapping.channelId}>当前绑定渠道（不可用）</option>
      )}
      {availableChannels.map((channel) => (
        <option key={channel.channelId} value={channel.channelId}>
          {channel.channelName} ({channel.channelType})
        </option>
      ))}
    </select>
  )

  const renderThinkingSelect = (index: number, mapping: ModelMapping, className = 'h-8') => (
    <select
      value={mapping.thinkingLevel || ''}
      onChange={(e) => handleChange(index, 'thinkingLevel', e.target.value)}
      className={`${className} w-full rounded-md border border-input bg-background px-2 text-sm ring-offset-background focus:outline-none focus:ring-2 focus:ring-ring`}
    >
      {THINKING_LEVELS.map((level) => (
        <option key={level.value} value={level.value}>
          {level.label}
        </option>
      ))}
    </select>
  )

  return (
    <div className="space-y-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="space-y-1">
          <Label>映射规则</Label>
          <p className="text-sm text-muted-foreground">按规则匹配模型与渠道。</p>
        </div>
        <Button type="button" size="sm" onClick={handleAdd}>
          + 添加映射
        </Button>
      </div>

      {mappings.length === 0 ? (
        <div className="rounded-md border border-dashed p-4 text-center text-sm text-muted-foreground">
          暂无模型映射，点击上方按钮添加
        </div>
      ) : (
        <>
          <div className="space-y-3 md:hidden">
            {mappings.map((mapping, index) => (
              <div key={index} className="rounded-xl border border-border/70 px-4 py-4">
                <div className="flex items-start justify-between gap-3">
                  <div className="font-medium">规则 {index + 1}</div>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    onClick={() => handleRemove(index)}
                    className="text-destructive hover:text-destructive"
                  >
                    删除
                  </Button>
                </div>

                <div className="mt-3 space-y-3">
                  <div className="space-y-2">
                    <Label className="text-xs text-muted-foreground">From</Label>
                    <Input
                      value={mapping.from}
                      onChange={(e) => handleChange(index, 'from', e.target.value)}
                      placeholder="源模型名"
                      className="h-9"
                    />
                  </div>
                  <div className="space-y-2">
                    <Label className="text-xs text-muted-foreground">To</Label>
                    {renderToField(index, mapping, 'h-9')}
                  </div>
                  <div className="space-y-2">
                    <Label className="text-xs text-muted-foreground">目标渠道</Label>
                    {renderChannelSelect(index, mapping, 'h-9')}
                  </div>
                  <div className="space-y-2">
                    <Label className="text-xs text-muted-foreground">思维强度</Label>
                    {renderThinkingSelect(index, mapping, 'h-9')}
                  </div>
                </div>

                <div className="mt-3 grid grid-cols-2 gap-3 text-sm">
                  <label className="flex items-center justify-between rounded-lg border border-border/70 px-3 py-2">
                    <span>伪非流</span>
                    <Checkbox
                      checked={mapping.pseudoNonStream || false}
                      onCheckedChange={(checked) => handleChange(index, 'pseudoNonStream', !!checked)}
                    />
                  </label>
                  <label className="flex items-center justify-between rounded-lg border border-border/70 px-3 py-2">
                    <span>仅AMP</span>
                    <Checkbox
                      checked={mapping.ampOnly || false}
                      onCheckedChange={(checked) => handleChange(index, 'ampOnly', !!checked)}
                    />
                  </label>
                  <label className="flex items-center justify-between rounded-lg border border-border/70 px-3 py-2">
                    <span>Fast</span>
                    <Checkbox
                      checked={mapping.fastMode || false}
                      onCheckedChange={(checked) => handleChange(index, 'fastMode', !!checked)}
                    />
                  </label>
                  <label className="flex items-center justify-between rounded-lg border border-border/70 px-3 py-2">
                    <span>指令</span>
                    <Checkbox
                      checked={mapping.customInstructionsEnabled || false}
                      onCheckedChange={(checked) => handleChange(index, 'customInstructionsEnabled', !!checked)}
                    />
                  </label>
                  <label className="col-span-2 flex items-center justify-between rounded-lg border border-border/70 px-3 py-2">
                    <span>Regex</span>
                    <Checkbox
                      checked={mapping.regex}
                      onCheckedChange={(checked) => handleChange(index, 'regex', !!checked)}
                    />
                  </label>
                </div>

                {mapping.pseudoNonStream && (
                  <div className="mt-3 space-y-2">
                    <Label className="text-xs text-muted-foreground">审计关键词</Label>
                    <Textarea
                      value={(mapping.auditKeywords || []).join('\n')}
                      onChange={(e) => {
                        const keywords = e.target.value
                          .split('\n')
                          .map(k => k.trim())
                          .filter(k => k !== '')
                        handleChange(index, 'auditKeywords', keywords)
                      }}
                      placeholder="每行一个关键词"
                      className="h-24 text-xs font-mono"
                    />
                  </div>
                )}

                {mapping.customInstructionsEnabled && (
                  <div className="mt-3 space-y-2">
                    <Label className="text-xs text-muted-foreground">自定义指令</Label>
                    <Textarea
                      value={mapping.customInstructions || ''}
                      onChange={(e) => handleChange(index, 'customInstructions', e.target.value)}
                      placeholder="输入自定义 instructions"
                      className="h-24 text-xs font-mono"
                    />
                  </div>
                )}
              </div>
            ))}
          </div>

          <div className="hidden rounded-md border md:block">
            <Table className="w-full table-fixed">
            <TableHeader>
              <TableRow>
                <TableHead className="w-[11rem] whitespace-nowrap align-middle">From</TableHead>
                <TableHead className="w-[12rem] whitespace-nowrap align-middle">To</TableHead>
                <TableHead className="w-[10rem] whitespace-nowrap align-middle">目标渠道</TableHead>
                <TableHead className="w-[7rem] whitespace-nowrap text-center align-middle">思维强度</TableHead>
                <TableHead className="w-[4rem] whitespace-nowrap text-center align-middle">伪非流</TableHead>
                <TableHead className="w-[4rem] whitespace-nowrap text-center align-middle">仅AMP</TableHead>
                <TableHead className="w-[4rem] whitespace-nowrap text-center align-middle">Fast</TableHead>
                <TableHead className="w-[4rem] whitespace-nowrap text-center align-middle">指令</TableHead>
                <TableHead className="w-[4rem] whitespace-nowrap text-center align-middle">Regex</TableHead>
                <TableHead className="w-[5rem] whitespace-nowrap text-center align-middle">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {mappings.map((mapping, index) => (
                <React.Fragment key={index}>
                <TableRow className="align-top">
                  <TableCell className="align-top">
                    <Input
                      value={mapping.from}
                      onChange={(e) => handleChange(index, 'from', e.target.value)}
                      placeholder="源模型名"
                      className="h-8"
                    />
                  </TableCell>
                  <TableCell className="align-top">
                    {renderToField(index, mapping)}
                  </TableCell>
                  <TableCell className="align-top">
                    {renderChannelSelect(index, mapping)}
                  </TableCell>
                  <TableCell className="align-top">
                    {renderThinkingSelect(index, mapping)}
                  </TableCell>
                  <TableCell className="pt-3 text-center align-top">
                    <Checkbox
                      checked={mapping.pseudoNonStream || false}
                      onCheckedChange={(checked) => handleChange(index, 'pseudoNonStream', !!checked)}
                    />
                  </TableCell>
                  <TableCell className="pt-3 text-center align-top">
                    <Checkbox
                      checked={mapping.ampOnly || false}
                      onCheckedChange={(checked) => handleChange(index, 'ampOnly', !!checked)}
                    />
                  </TableCell>
                  <TableCell className="pt-3 text-center align-top">
                    <Checkbox
                      checked={mapping.fastMode || false}
                      onCheckedChange={(checked) => handleChange(index, 'fastMode', !!checked)}
                    />
                  </TableCell>
                  <TableCell className="pt-3 text-center align-top">
                    <Checkbox
                      checked={mapping.customInstructionsEnabled || false}
                      onCheckedChange={(checked) => handleChange(index, 'customInstructionsEnabled', !!checked)}
                    />
                  </TableCell>
                  <TableCell className="pt-3 text-center align-top">
                    <Checkbox
                      checked={mapping.regex}
                      onCheckedChange={(checked) => handleChange(index, 'regex', !!checked)}
                    />
                  </TableCell>
                  <TableCell className="pt-2 text-center align-top">
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      onClick={() => handleRemove(index)}
                      className="text-destructive hover:text-destructive"
                    >
                      删除
                    </Button>
                  </TableCell>
                </TableRow>
                {mapping.pseudoNonStream && (
                  <TableRow key={`${index}-audit`}>
                    <TableCell colSpan={10} className="pt-0">
                      <div className="flex items-start gap-2 pb-2">
                        <Label className="text-xs text-muted-foreground whitespace-nowrap pt-2">审计关键词:</Label>
                        <Textarea
                          value={(mapping.auditKeywords || []).join('\n')}
                          onChange={(e) => {
                            const keywords = e.target.value
                              .split('\n')
                              .map(k => k.trim())
                              .filter(k => k !== '')
                            handleChange(index, 'auditKeywords', keywords)
                          }}
                          placeholder="每行一个关键词，留空使用默认列表（中文垃圾/赌博词汇）"
                          className="h-20 text-xs font-mono"
                        />
                      </div>
                    </TableCell>
                  </TableRow>
                )}
                {mapping.customInstructionsEnabled && (
                  <TableRow key={`${index}-instructions`}>
                    <TableCell colSpan={10} className="pt-0">
                      <div className="flex items-start gap-2 pb-2">
                        <Label className="text-xs text-muted-foreground whitespace-nowrap pt-2">自定义指令:</Label>
                        <Textarea
                          value={mapping.customInstructions || ''}
                          onChange={(e) => handleChange(index, 'customInstructions', e.target.value)}
                          placeholder="输入自定义 instructions 内容，将注入或替换请求中的 instructions / system message"
                          className="h-24 text-xs font-mono"
                        />
                      </div>
                    </TableCell>
                  </TableRow>
                )}
                </React.Fragment>
              ))}
            </TableBody>
            </Table>
          </div>
        </>
      )}

      <div className="text-xs text-muted-foreground">
        开启伪非流或指令后，会在当前规则下展开附加输入区。
      </div>
    </div>
  )
}
