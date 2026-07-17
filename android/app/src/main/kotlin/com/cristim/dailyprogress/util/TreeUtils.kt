package com.cristim.dailyprogress.util

import com.cristim.dailyprogress.model.TreeTaskDto

/**
 * Returns this task and all of its descendants in pre-order (task first, then
 * each child's full subtree). Handles arbitrary nesting depth.
 */
fun TreeTaskDto.flattenPreOrder(): List<TreeTaskDto> = buildList {
    add(this@flattenPreOrder)
    children.forEach { child -> addAll(child.flattenPreOrder()) }
}
