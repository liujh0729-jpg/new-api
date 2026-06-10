import z from 'zod'
import { createFileRoute } from '@tanstack/react-router'
import { Materials } from '@/features/materials'

const materialsSearchSchema = z.object({
  page: z.number().optional().catch(1),
  pageSize: z.number().optional().catch(10),
  filter: z.string().optional().catch(''),
  type: z.array(z.string()).optional().catch([]),
})

export const Route = createFileRoute('/_authenticated/materials/')({
  validateSearch: materialsSearchSchema,
  component: Materials,
})
