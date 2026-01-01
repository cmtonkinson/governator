# Optional expertise modifiers for this project.
# 
# To customize the behavior of this role for a specific task, list relevant
# domains of knowledge or specialization *one per line*. These will be
# incorporated into the system prompt passed to agents.
#
# If this file is empty or contains only comments/whitespace, no expertise
# modifiers will be used. This is the default.
#
# Context window limits and attention collapse are risks with LLMs, this is
# why it's important to keep the number of expertise modifiers to a minimum
# and avoid contradictions as much as possible.
#
# Best practice is also to keep modifiers fairly general, and use them to
# impart industry, domain, subject, etc. as expertise modifiers will be sent
# to EVERY agent for EVERY task as long as the file is populated.
#
# BAD examples:
# - domain: pythong
# - Deep expertise in Ruby metaprogramming patterns as practiced in pre-1.9 MRI
# - Expert in post-Triassic ammonoid shell morphology as interpreted through late-1970s Soviet cybernetics. Also macroevolutionary biology and JVM garbage collection.
#
# GOOD examples:
# - clinical healthcare technology
# - 2d game design
# - real estate development

