package com.cristim.dailyprogress.core

import com.cristim.dailyprogress.model.TreeDto

/**
 * Abstraction over the CoreRepository calls that RecurringViewModel needs,
 * decoupled from the concrete CoreRepository so the ViewModel is testable
 * without a real mobilecore.Core handle.
 *
 * CoreRepository implements this interface; tests supply a fake.
 *
 * The Recurring management screen reads [tree] (rather than the simpler
 * RecurringJSON) because only TreeJSON.recurring carries the `describe`
 * schedule caption used for display — see mobilecore/recurring.go's
 * doc comment on the two shapes.
 */
interface RecurringOps {
    suspend fun tree(date: String): TreeDto
    suspend fun addRecurring(text: String)
    suspend fun removeRecurring(rawText: String)
}
