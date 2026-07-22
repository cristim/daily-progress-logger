package com.cristim.dailyprogress.core

import com.cristim.dailyprogress.model.EveningDecisionsDto
import com.cristim.dailyprogress.model.MorningCandidateDto
import com.cristim.dailyprogress.model.MorningDecisionsDto
import com.cristim.dailyprogress.model.TreeDto
import com.cristim.dailyprogress.model.WeeklyPlanDto

/**
 * Abstraction over the CoreRepository calls that CheckinViewModel needs,
 * decoupled from the concrete CoreRepository so the ViewModel is testable
 * without a real mobilecore.Core handle.
 *
 * CoreRepository implements this interface; tests supply a fake.
 */
interface CheckinOps : DailyPromptOps {
    suspend fun morningCandidates(date: String): List<MorningCandidateDto>
    suspend fun applyMorning(date: String, decisions: MorningDecisionsDto)
    suspend fun weeklyPlan(date: String): WeeklyPlanDto
    suspend fun tree(date: String): TreeDto
    suspend fun applyEvening(date: String, decisions: EveningDecisionsDto)
}
