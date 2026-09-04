// Top-level statements, so a local function is declared with no enclosing member to name it
// after. It still gets a span, under its bare local name.
void Helper(int a)
{
    if (a > 0)
    {
        System.Console.WriteLine(a);
    }
}

Helper(1);
