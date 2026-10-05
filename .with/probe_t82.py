#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Probe the exact T8.2 line characters around the head to fix matcher."""
import io

PATH = r"D:/workspace/GitHub/relkit/docs/_pending/2026-10-04-ci-release-stability-plan.md"

with io.open(PATH, "r", encoding="utf-8") as fh:
    for line in fh:
        if line.startswith("- T8.2 "):
            head = line[:120]
            print(repr(head))
            break
