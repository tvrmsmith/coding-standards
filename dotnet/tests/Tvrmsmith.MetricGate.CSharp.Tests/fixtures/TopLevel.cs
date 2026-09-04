// Top-level statements, so a local function is declared with no enclosing member to name it
// after. It still gets a span, under its bare local name. The global statements around it get
// none, because they belong to a synthesized <Main>$ that has no declaration syntax, so the
// branch below is scored nowhere. docs/csharp-decision-points.md records that as a deferral.
void Helper(int a)
{
    if (a > 0)
    {
        System.Console.WriteLine(a);
    }
}

if (args.Length > 0)
{
    Helper(args.Length);
}
