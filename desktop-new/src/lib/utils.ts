import { type ClassValue, clsx } from "clsx"
import { twMerge } from "tailwind-merge"
import { tv } from "tailwind-variants"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

export type WithElementRef<T, E extends HTMLElement = HTMLElement> = T & {
  ref?: E | null
}

export type WithoutChildren<T> = Omit<T, "children">
export type WithoutChildrenOrChild<T> = Omit<T, "children" | "child">
export type WithoutChild<T> = Omit<T, "child">

export { tv }
