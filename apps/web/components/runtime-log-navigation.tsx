'use client';
import {useEffect} from 'react';
import {runtimeLogBrowserGuard} from '../lib/runtime-logs';
export function RuntimeLogNavigation(){useEffect(runtimeLogBrowserGuard,[]);return null}
