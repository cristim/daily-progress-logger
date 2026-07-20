package com.cristim.dailyprogress.core

import com.cristim.dailyprogress.model.PendingWeekDto
import com.cristim.dailyprogress.model.ReviewDecisionsDto
import com.cristim.dailyprogress.model.WeekReviewCandidatesDto

/**
 * Abstraction over the three CoreRepository calls that WeekReviewViewModel
 * needs, decoupled from the concrete CoreRepository so the ViewModel is
 * testable without a real mobilecore.Core handle.
 *
 * CoreRepository implements this interface; tests supply a fake.
 */
interface WeekReviewOps {
    suspend fun unreviewedWeek(date: String): PendingWeekDto
    suspend fun weekReviewCandidates(date: String): WeekReviewCandidatesDto
    suspend fun applyWeekReview(date: String, decisions: ReviewDecisionsDto)
}
