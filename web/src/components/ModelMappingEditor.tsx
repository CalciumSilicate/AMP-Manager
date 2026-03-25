import React, { useState, useEffect, useRef } from 'react'
import { ModelMapping } from '../api/amp'
import { listAvailableModels, AvailableModel } from '../api/models'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Checkbox } from '@/components/ui/checkbox'
import { Card, CardContent } from '@/components/ui/card'
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
  const dropdownRefs = useRef<(HTMLTableCellElement | null)[]>([])

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

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <Label>Model Mappings</Label>
        <Button type="button" size="sm" onClick={handleAdd}>
          + 添加映射
        </Button>
      </div>

      {mappings.length === 0 ? (
        <div className="rounded-md border border-dashed p-4 text-center text-sm text-muted-foreground">
          暂无模型映射，点击上方按钮添加
        </div>
      ) : (
        <div className="rounded-md border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>From</TableHead>
                <TableHead>To</TableHead>
                <TableHead>目标渠道</TableHead>
                <TableHead className="text-center">思维强度</TableHead>
                <TableHead className="text-center">伪非流</TableHead>
                <TableHead className="text-center">仅AMP</TableHead>
                <TableHead className="text-center">Fast</TableHead>
                <TableHead className="text-center">指令</TableHead>
                <TableHead className="text-center">Regex</TableHead>
                <TableHead className="text-center">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {mappings.map((mapping, index) => (
                <React.Fragment key={index}>
                <TableRow>
                  <TableCell>
                    <Input
                      value={mapping.from}
                      onChange={(e) => handleChange(index, 'from', e.target.value)}
                      placeholder="源模型名"
                      className="h-8"
                    />
                  </TableCell>
                  <TableCell 
                    className="relative"
                    ref={el => { dropdownRefs.current[index] = el }}
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
                        className="h-8"
                      />
                      {availableModels.length > 0 && (
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          onClick={() => showDropdown === index ? setShowDropdown(null) : openDropdown(index)}
                        >
                          ▼
                        </Button>
                      )}
                    </div>
                    
                    {showDropdown === index && (
                      <div 
                        id="model-dropdown"
                        className="fixed z-50 max-h-80 w-96 overflow-auto rounded-md border bg-popover shadow-xl"
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
                  </TableCell>
                  <TableCell>
                    <select
                      value={mapping.channelId || ''}
                      onChange={(e) => handleChange(index, 'channelId', e.target.value)}
                      className="h-8 w-full rounded-md border border-input bg-background px-2 text-sm ring-offset-background focus:outline-none focus:ring-2 focus:ring-ring"
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
                  </TableCell>
                  <TableCell>
                    <select
                      value={mapping.thinkingLevel || ''}
                      onChange={(e) => handleChange(index, 'thinkingLevel', e.target.value)}
                      className="h-8 w-full rounded-md border border-input bg-background px-2 text-sm ring-offset-background focus:outline-none focus:ring-2 focus:ring-ring"
                    >
                      {THINKING_LEVELS.map((level) => (
                        <option key={level.value} value={level.value}>
                          {level.label}
                        </option>
                      ))}
                    </select>
                  </TableCell>
                  <TableCell className="text-center">
                    <Checkbox
                      checked={mapping.pseudoNonStream || false}
                      onCheckedChange={(checked) => handleChange(index, 'pseudoNonStream', !!checked)}
                    />
                  </TableCell>
                  <TableCell className="text-center">
                    <Checkbox
                      checked={mapping.ampOnly || false}
                      onCheckedChange={(checked) => handleChange(index, 'ampOnly', !!checked)}
                    />
                  </TableCell>
                  <TableCell className="text-center">
                    <Checkbox
                      checked={mapping.fastMode || false}
                      onCheckedChange={(checked) => handleChange(index, 'fastMode', !!checked)}
                    />
                  </TableCell>
                  <TableCell className="text-center">
                    <Checkbox
                      checked={mapping.customInstructionsEnabled || false}
                      onCheckedChange={(checked) => handleChange(index, 'customInstructionsEnabled', !!checked)}
                    />
                  </TableCell>
                  <TableCell className="text-center">
                    <Checkbox
                      checked={mapping.regex}
                      onCheckedChange={(checked) => handleChange(index, 'regex', !!checked)}
                    />
                  </TableCell>
                  <TableCell className="text-center">
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
      )}

      <Card className="bg-muted/50">
        <CardContent className="p-3 text-xs text-muted-foreground">
          <p><strong>说明:</strong></p>
          <ul className="mt-1 list-inside list-disc space-y-1">
            <li><strong>From:</strong> 请求中的模型名称（支持正则表达式）</li>
            <li><strong>To:</strong> 映射到的目标模型（可从列表选择或手动输入）</li>
            <li><strong>目标渠道:</strong> 可选，指定后优先路由到该渠道；留空时保持自动选择</li>
            <li><strong>思维强度:</strong> 设置模型的推理/思考强度 (low/medium/high/xhigh)</li>
            <li><strong>伪非流:</strong> 以流式请求上游，但完整接收后才返回给客户端（用于响应审查）</li>
            <li><strong>仅AMP:</strong> 仅当请求来自 AMP 客户端时才应用此映射（检测 X-Amp-Feature 请求头）</li>
            <li><strong>Fast:</strong> 为 GPT 模型启用快速模式，设置 service_tier 为 priority</li>
            <li><strong>指令:</strong> 自定义 instructions，启用后可注入或替换请求中的 system message / instructions 字段</li>
            <li><strong>审计关键词:</strong> 伪非流启用时，额外检测的自定义关键词（每行一个，留空使用默认列表）</li>
            <li><strong>Regex:</strong> 是否将 From 字段作为正则表达式匹配</li>
          </ul>
        </CardContent>
      </Card>
    </div>
  )
}
