# overlord-ipa
A FreeIPA "Overlord" which can monitor, update, and collect information about enrolled Linux (currently) systems with a pretty dashboard with integrated FreeIPA logon!

## Go Coding Rules

Mandatory coding guidelines for Go:

1. Avoid `:=` as much as possible, only use in loops or select statements
2. Use explicit type declarations for variables in blocks
3. Use named/naked returns in functions to improve readability
4. For all major functions, put a brief one-line comment explaining the function's purpose. If it needs more than a line, expand as needed. Do not over document
5. New lines after } when sensible, new lines to separate logical blocks of code
6. For checking errors, put the function call into the if statement
7. Use grouped `var (...)` declarations when a block needs more than one local variable
8. For comma-ok checks, put the assignment in the if statement when the value is only needed for that branch or immediate check
9. Avoid redundant `return` statements inside conditionals when the function can naturally fall through to its named return

Here's an example:

```go
var (
    count, other int = 1, 3
    something *reflect.Value
    err error
)

// Computes the sum of two integers, returning an error if the result is negative.
func doSomething(a, b int) (c int, err error) {
    if c = a + b; c < 0 {
        err = fmt.Errorf("result is negative")
        c = 0
        return
    }

    return
}


func main() {
    if count, err = doSomething(1, 2); err != nil {
        fmt.Printf("Error: %v\n", err)
    } else {
        fmt.Printf("Result: %d\n", count)
    }

    something = nil
}
```
