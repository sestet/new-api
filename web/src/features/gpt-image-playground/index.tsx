import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'

import ConfirmDialog from './external/components/ConfirmDialog'
import DetailModal from './external/components/DetailModal'
import {
  FavoriteCollectionPickerModal,
  FavoriteCollectionsView,
  ManageCollectionsModal,
} from './external/components/FavoriteCollections'
import ImageContextMenu from './external/components/ImageContextMenu'
import InputBar from './external/components/InputBar'
import Lightbox from './external/components/Lightbox'
import MaskEditorModal from './external/components/MaskEditorModal'
import SearchBar from './external/components/SearchBar'
import SupportPromptModal from './external/components/SupportPromptModal'
import TaskGrid from './external/components/TaskGrid'
import Toast from './external/components/Toast'
import { useGlobalClickSuppression } from './external/lib/clickSuppression'
import { initStore, useStore } from './external/store'
import { gptImagePlaygroundLayoutClasses } from './layout'
import { PlaygroundTokenSelector } from './token-selector'

import './styles.css'

export function GptImagePlayground() {
  const { t } = useTranslation()
  const appMode = useStore((state) => state.appMode)
  const filterFavorite = useStore((state) => state.filterFavorite)
  const activeFavoriteCollectionId = useStore(
    (state) => state.activeFavoriteCollectionId
  )

  useGlobalClickSuppression()

  useEffect(() => {
    void initStore()
    const preventPageImageDrag = (event: DragEvent) => {
      if ((event.target as HTMLElement | null)?.closest('img')) {
        event.preventDefault()
      }
    }
    document.addEventListener('dragstart', preventPageImageDrag)
    return () => document.removeEventListener('dragstart', preventPageImageDrag)
  }, [])

  return (
    <div className='gpt-image-playground bg-background text-foreground relative flex size-full min-h-0 flex-col overflow-hidden'>
      <header className='safe-area-top border-border flex min-h-14 shrink-0 items-center justify-between gap-3 border-b px-4'>
        <div className='min-w-0'>
          <h1 className='truncate text-base font-semibold'>
            {t('Image Playground')}
          </h1>
          <p className='text-muted-foreground hidden text-xs sm:block'>
            {t(
              'Generate and edit images through your New API channels.'
            )}
          </p>
        </div>
        <PlaygroundTokenSelector />
      </header>

      <main className={gptImagePlaygroundLayoutClasses.taskSurface}>
        <div className={gptImagePlaygroundLayoutClasses.taskContent}>
          <SearchBar />
          {appMode === 'gallery' &&
            (filterFavorite && !activeFavoriteCollectionId ? (
              <FavoriteCollectionsView />
            ) : (
              <TaskGrid />
            ))}
        </div>
      </main>

      <InputBar />
      <DetailModal />
      <Lightbox />
      <ConfirmDialog />
      <SupportPromptModal />
      <FavoriteCollectionPickerModal />
      <ManageCollectionsModal />
      <Toast />
      <MaskEditorModal />
      <ImageContextMenu />
    </div>
  )
}
