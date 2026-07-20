package com.cristim.dailyprogress.core

import com.cristim.dailyprogress.model.BacklogDto

/**
 * Abstraction over the CoreRepository calls that BacklogViewModel needs,
 * decoupled from the concrete CoreRepository so the ViewModel is testable
 * without a real mobilecore.Core handle.
 *
 * CoreRepository implements this interface; tests supply a fake.
 */
interface BacklogOps {
    suspend fun backlog(): BacklogDto
    suspend fun adoptFromBacklog(date: String, text: String)
    suspend fun moveBacklogItem(text: String, toNextWeek: Boolean)
}
