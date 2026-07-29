import { createFileRoute, redirect } from '@tanstack/react-router'

import { Main } from '@/components/layout'
import { GptImagePlayground } from '@/features/gpt-image-playground'
import { isSidebarModuleEnabled } from '@/lib/nav-modules'

export const Route = createFileRoute('/_authenticated/gpt-image-playground/')({
  beforeLoad: () => {
    if (!isSidebarModuleEnabled('chat', 'gptImagePlayground')) {
      throw redirect({ to: '/dashboard' })
    }
  },
  component: GptImagePlaygroundPage,
})

function GptImagePlaygroundPage() {
  return (
    <Main className='p-0'>
      <GptImagePlayground />
    </Main>
  )
}
